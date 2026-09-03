package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/JaredTate/coeus/internal/contract"
)

// runLock is the file that stops a second copy of the agent from running on one
// home folder. The lock is held by the operating system on the open file, so a
// copy that is killed outright still lets the next one start, and a home folder
// left behind by a crash needs no cleaning up.
//
// It lives in a file of its own because serve.go is already the one place every
// package meets, and the rule that a file has one job is what keeps that file
// readable.
type runLock struct {
	path string
	file *os.File
}

// takeRunLock takes the lock, refusing when another copy of the agent holds it.
func takeRunLock(path string) (*runLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, contract.SecretFileMode)
	if err != nil {
		return nil, fmt.Errorf("cannot open the lock file %s, so check that the run folder is there and this account can write in it: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("another copy of coeus is already running on this home folder, because something holds the lock file %s, so stop that one first: %w", path, err)
	}

	// The number of the process holding the lock is written into the file, so
	// that a person who finds the file can see which process to stop.
	if err := file.Truncate(0); err == nil {
		_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &runLock{path: path, file: file}, nil
}

// release gives the lock up. The file itself is left where it is, because
// removing it would take the lock away from whoever opened it next.
func (lock *runLock) release() error {
	unlocked := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	return errors.Join(unlocked, lock.file.Close())
}
