package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/manifoldco/promptui"
)

type Disposer func() error

const (
	GameRGSS3A       = "Game.rgss3a"
	GameRGSS3ABackup = "Game.rgss3a~"
	ScriptsRVDARA2   = "Scripts.rvdata2"
	SceneBaseRB      = "Scene_Base.rb"
	CheatScriptRB    = "AsCheater.rb"
)

const (
	SceneBaseInjectionScript = "    AsCheater.update"
)

//go:embed AsCheater.rb
var CheatScript string

// RPGMakerDecrypterCLIBin https://github.com/uuksu/RPGMakerDecrypter/releases
//
//go:embed RPGMakerDecrypter-cli.exe
var RPGMakerDecrypterCLIBin []byte

//go:embed rvdata2-unpacker/rvunpacker.exe
var RVUnpackerBin []byte

func pause() {
	log.Println("Press [Enter] to exit...")
	var input string
	_, _ = fmt.Scanln(&input)
}

func Println(args ...any) {
	log.Println(args...)
}

func Fatalln(args ...any) {
	log.Println(args...)
	pause()
	os.Exit(1)
}

func createExe(name string, data []byte) (string, Disposer) {
	Println("Creating", name, "...")
	file, err := os.CreateTemp(os.TempDir(), name)
	if err != nil {
		Fatalln("Failed to create", name, ":", err)
	}
	defer func() {
		_ = file.Close()
	}()
	_, err = file.Write(data)
	if err != nil {
		Fatalln("Failed to write", name, ":", err)
	}
	_ = file.Close()

	Println("Created", name, "at", file.Name())

	return file.Name(), func() error {
		return os.Remove(file.Name())
	}
}

func run(name, cwd string, arg ...string) {
	cmd := exec.Command(name, arg...)
	cmd.Dir = cwd

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		Println(name, ":\n", string(output))
	}
	if err != nil {
		Fatalln("Failed to run", name, ":", err)
	}
}

func main() {
	if runtime.GOOS != "windows" {
		Fatalln("This program only runs on Windows")
	}

	// print credits
	Println("Credits:")
	Println("RPGMakerDecrypter-cli.exe by https://github.com/uuksu/RPGMakerDecrypter")
	Println("rvunpacker.exe by https://www.gamedev.net/forums/topic/646333-rpg-maker-vx-ace-data-conversion-utility/")

	Println()
	Println("If this program failed to patch your game, please do it manually with the instructions in README.md.")

	Println()

	// replace `CheatScript` if a AsCheater.rb exists in current folder
	if _, err := os.Stat(CheatScriptRB); err == nil {
		Println("Using", CheatScriptRB, "in current folder")
		bs, err := os.ReadFile(CheatScriptRB)
		if err != nil {
			Fatalln("Failed to read", CheatScriptRB, ":", err)
		}
		CheatScript = string(bs)
	}

	// Parse root from args
	root := "."
	if args := flag.Args(); len(args) > 0 {
		root = args[0]
	}

	Println("Ready to patch Game.exe at", root)

	stat, err := os.Stat(root)
	if err != nil {
		Fatalln("Failed to stat", root, ":", err)
	} else if !stat.IsDir() {
		Fatalln(root, "is not a directory")
	}

	// Parse for commandline flags for operations.
	doPatch := flag.Bool("patch", false, "Patch game")
	doRestore := flag.Bool("restore", false, "Restore game")
	doRepatch := flag.Bool("repatch", false, "Restore then patch game")
	flag.Parse()

    if *doPatch || *doRestore || *doRepatch {
		switch {
		case *doPatch:
			patch(root)
		case *doRestore:
			restoreGame(root)
		case *doRepatch:
			rgss3 := path.Join(root, GameRGSS3ABackup)
			if _, err := os.Stat(rgss3); err == nil {
				restoreGame(root)
			} else {
				Println("Failed to find", rgss3, ", nothing to restore, start patching straight away")
			}
			patch(root)
		}
		return
	}


	selections := []string{
		"Patch this game(default)",    // 0
		"Restore game",                // 1
		"Re-patch(Restore and Patch)", // 2
	}

	prompt := promptui.Select{
		Label: "Select a patching operation",
		Items: selections,
	}
	index, _, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return
	}

	switch index {
	case 0:
		patch(root)
	case 1:
		restoreGame(root)
	case 2:
		rgss3 := path.Join(root, GameRGSS3ABackup)
		_, err := os.Stat(rgss3)
		if err != nil {
			Println("Failed to find", rgss3, ", nothing to restore, start patching straight away")
		} else {
			restoreGame(root)
		}
		patch(root)
	}

	pause()
}

func patch(root string) {
	dec, decDis := createExe("RPGMAC-RPGMakerDecrypter-cli.exe", RPGMakerDecrypterCLIBin)
	rvu, rvuDis := createExe("RPGMAC-rvunpacker.exe", RVUnpackerBin)

	scriptsRVDARA2 := path.Join(root, "Data", ScriptsRVDARA2)
	_, err := os.Stat(scriptsRVDARA2)
	if err != nil {
		unzipGame(dec, root)
		injectSceneUpdate(rvu, root)
	} else {
		Println("Game already unzipped")
		injectSceneUpdate(rvu, root)
	}

	defer func() {
		_ = decDis()
		_ = rvuDis()
	}()

	Println("Patched Game.exe at", root)
}

func injectSceneUpdate(exe, root string) {
	Println("Patching Game.exe...")

	// decode Scripts.rvdata2
	run(exe, root, "decode", root)

	sceneBaseRb := path.Join(root, "Scripts", SceneBaseRB)

	if _, err := os.Stat(sceneBaseRb); err != nil {
		Fatalln("Failed to find", sceneBaseRb)
	}

	sceneBaseText, err := os.ReadFile(sceneBaseRb)
	if err != nil {
		Fatalln("Failed to read", sceneBaseRb, ":", err)
	}

	if strings.Contains(string(sceneBaseText), CheatScript) {
		Fatalln("Game already patched")
	}

	sceneBaseLines := strings.Split(string(sceneBaseText), "\r\n")
	injectPoint := -1
	for i, line := range sceneBaseLines {
		if strings.TrimSpace(line) == "def update" {
			injectPoint = i
			break
		}
	}
	if injectPoint < 0 {
		Fatalln("Failed to find 'def update' in", sceneBaseRb)
	}

	sceneBaseInjectedText := CheatScript + "\r\n\r\n"

	sceneBaseInjectedText += strings.Join(sceneBaseLines[:injectPoint+1], "\r\n")
	sceneBaseInjectedText += "\r\n" + SceneBaseInjectionScript + "\r\n"
	sceneBaseInjectedText += strings.Join(sceneBaseLines[injectPoint+1:], "\r\n")
	err = os.WriteFile(sceneBaseRb, []byte(sceneBaseInjectedText+"\r\n"), 0644)
	if err != nil {
		Fatalln("Failed to write", sceneBaseRb, ":", err)
	}

	// encode Scripts.rvdata2
	run(exe, root, "encode", root)
}

func unzipGame(exe, root string) {
	Println("Unzipping Game.exe...")
	src := path.Join(root, GameRGSS3A)
	dst := path.Join(root, GameRGSS3ABackup)
	run(exe, root, src)

	Println("Renaming", src, "to", dst)
	err := os.Rename(src, dst)
	if err != nil {
		Fatalln("Failed to rename", src, "to", dst, ":", err)
	}
}

func restoreGame(root string) {
	src := path.Join(root, GameRGSS3ABackup)
	dis := path.Join(root, GameRGSS3A)
	_, err := os.Stat(src)
	if err != nil {
		Fatalln("Failed to find", src, ", nothing to restore")
	}

	scriptsPath := path.Join(root, "Scripts")
	err = os.RemoveAll(scriptsPath)
	if err != nil {
		Fatalln("Failed to remove", scriptsPath, ":", err)
	}

	yamlPath := path.Join(root, "YAML")
	err = os.RemoveAll(yamlPath)
	if err != nil {
		Fatalln("Failed to remove", yamlPath, ":", err)
	}

	scriptsRvdata2 := path.Join(root, "Data", ScriptsRVDARA2)
	err = os.Remove(scriptsRvdata2)
	if err != nil {
		Fatalln("Failed to remove", scriptsRvdata2, ":", err)
	}

	err = os.Rename(src, dis)
	if err != nil {
		Fatalln("Failed to rename", src, "to", dis, ":", err)
	}

	Println("Restored Game.exe at", root)
}
