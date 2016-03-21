package assemble

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

const (
	shellBin = "/bin/sh"
)

func Assemble(optBin, passPlugin, llcBin, asBin, ldBin, linkScript string, bitcode []byte) (elf []byte, err error) {
	objName, err := tempFileName("gateobj")
	if err != nil {
		return
	}
	defer os.Remove(objName)

	objCmd := &exec.Cmd{
		Path: shellBin,
		Args: []string{
			shellBin,
			"-c",
			fmt.Sprintf("%s -load=%s -gate | %s | %s -o %s", optBin, passPlugin, llcBin, asBin, objName),
		},
		Env: []string{},
		Dir: "/",
	}

	stdin, err := objCmd.StdinPipe()
	if err != nil {
		return
	}

	err = objCmd.Start()
	if err != nil {
		stdin.Close()
		return
	}

	writeLen, writeErr := stdin.Write(bitcode)
	stdin.Close()

	err = waitProcessError(objCmd)
	if err != nil {
		return
	}

	if writeLen < len(bitcode) {
		if writeErr != nil {
			err = writeErr
		} else {
			err = errors.New("bitcode not completely processed")
		}
		return
	}

	elfName, err := tempFileName("gateelf")
	if err != nil {
		return
	}
	defer os.Remove(elfName)

	elfCmd := &exec.Cmd{
		Path: ldBin,
		Args: []string{ldBin, "-T", linkScript, "-o", elfName, objName},
		Env:  []string{},
		Dir: "/",
	}

	err = elfCmd.Start()
	if err != nil {
		return
	}

	err = waitProcessError(elfCmd)
	if err != nil {
		return
	}

	elf, err = ioutil.ReadFile(elfName)
	return
}

func tempFileName(prefix string) (name string, err error) {
	f, err := ioutil.TempFile("", prefix)
	if err != nil {
		return
	}
	defer f.Close()
	name = f.Name()
	return
}

func waitProcessError(cmd *exec.Cmd) (err error) {
	err = cmd.Wait()
	if err != nil {
		return
	}

	if !cmd.ProcessState.Success() {
		err = errors.New(cmd.ProcessState.String())
		return
	}

	return
}
