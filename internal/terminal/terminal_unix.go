//go:build !windows

package terminal

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// unixTerminal 使用 golang.org/x/term 实现 Terminal，该库可在
// Linux、macOS 和 BSD 之间可移植地处理原始模式和尺寸查询。
type unixTerminal struct {
	in    *os.File
	out   *os.File
	inFd  int
	outFd int
}

// New 构造 Unix 终端后端。
func New() (Terminal, error) {
	return &unixTerminal{
		in:    os.Stdin,
		out:   os.Stdout,
		inFd:  int(os.Stdin.Fd()),
		outFd: int(os.Stdout.Fd()),
	}, nil
}

func (t *unixTerminal) Out() io.Writer { return t.out }
func (t *unixTerminal) In() io.Reader  { return t.in }

func (t *unixTerminal) Init() (func(), error) {
	state, err := term.MakeRaw(t.inFd)
	if err != nil {
		return nil, fmt.Errorf("terminal: make raw: %w", err)
	}
	restore := func() {
		_ = term.Restore(t.inFd, state)
	}
	return restore, nil
}

func (t *unixTerminal) Size() (int, int, error) {
	w, h, err := term.GetSize(t.outFd)
	if err != nil {
		return 0, 0, fmt.Errorf("terminal: get size: %w", err)
	}
	if w < 1 {
		w = 80
	}
	if h < 1 {
		h = 25
	}
	return w, h, nil
}
