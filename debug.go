// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

func firstSupported(kcfg config.KernelConfig, ka config.Artifact,
	kernel string) (ki config.KernelInfo, err error) {

	km, err := kernelMask(kernel)
	if err != nil {
		return
	}

	ka.SupportedKernels = []config.KernelMask{km}

	for _, ki = range kcfg.Kernels {
		var supported bool
		supported, err = ka.Supported(ki)
		if err != nil || supported {
			return
		}
	}

	err = errors.New("No supported kernel found")
	return
}

func handleLine(q *qemu.QemuSystem) (err error) {
	fmt.Print("out-of-tree> ")
	rawLine := "help"
	fmt.Scanf("%s", &rawLine)
	params := strings.Fields(rawLine)
	cmd := params[0]

	switch cmd {
	case "h", "help":
		fmt.Printf("help\t: print this help message\n")
		fmt.Printf("log\t: print qemu log\n")
		fmt.Printf("clog\t: print qemu log and cleanup buffer\n")
		fmt.Printf("cleanup\t: cleanup qemu log buffer\n")
		fmt.Printf("ssh\t: print arguments to ssh command\n")
		fmt.Printf("quit\t: quit\n")
	case "l", "log":
		fmt.Println(string(q.Stdout))
	case "cl", "clog":
		fmt.Println(string(q.Stdout))
		q.Stdout = []byte{}
	case "c", "cleanup":
		q.Stdout = []byte{}
	case "s", "ssh":
		fmt.Println(q.GetSshCommand())
	case "q", "quit":
		return errors.New("end of session")
	default:
		fmt.Println("No such command")
	}
	return
}

func interactive(q *qemu.QemuSystem) (err error) {
	for {
		err = handleLine(q)
		if err != nil {
			return
		}
	}
}

func debugHandler(kcfg config.KernelConfig, workPath, kernRegex, gdb string,
	dockerTimeout time.Duration) (err error) {

	ka, err := config.ReadArtifactConfig(workPath + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	if ka.SourcePath == "" {
		ka.SourcePath = workPath
	}

	ki, err := firstSupported(kcfg, ka, kernRegex)
	if err != nil {
		return
	}

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewQemuSystem(qemu.X86_64, kernel, ki.RootFS)
	if err != nil {
		return
	}
	q.Debug(gdb)
	coloredGdbAddress := aurora.BgGreen(aurora.Black(gdb))
	fmt.Printf("[*] gdb runned on %s\n", coloredGdbAddress)

	err = q.Start()
	if err != nil {
		return
	}
	defer q.Stop()

	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	outFile, output, err := build(tmp, ka, ki, dockerTimeout)
	if err != nil {
		log.Println(err, output)
		return
	}

	remoteFile := "/tmp/artifact"
	if ka.Type == config.KernelModule {
		remoteFile += ".ko"
	}

	err = q.CopyFile("user", outFile, remoteFile)
	if err != nil {
		return
	}

	coloredRemoteFile := aurora.BgGreen(aurora.Black(remoteFile))
	fmt.Printf("[*] build result copied to %s\n", coloredRemoteFile)

	err = interactive(q)
	return
}
