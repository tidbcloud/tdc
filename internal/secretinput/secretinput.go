package secretinput

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type stringReader interface {
	ReadString(byte) (string, error)
}

func IsTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func Read(ctx context.Context, prompt string, in io.Reader, out io.Writer, secret bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	type readResult struct {
		value string
		err   error
	}
	result := make(chan readResult, 1)
	go func() {
		value, err := read(in, out, secret)
		result <- readResult{value: value, err: err}
	}()
	select {
	case got := <-result:
		return got.value, got.err
	case <-ctx.Done():
		_, _ = fmt.Fprintln(out)
		return "", ctx.Err()
	}
}

func read(in io.Reader, out io.Writer, secret bool) (string, error) {
	if secret {
		if file, ok := in.(*os.File); ok && IsTerminal(file) {
			value, err := term.ReadPassword(int(file.Fd()))
			if err != nil {
				return "", err
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return "", err
			}
			return strings.TrimSpace(string(value)), nil
		}
	}
	if reader, ok := in.(stringReader); ok {
		value, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}
	reader := bufio.NewReader(in)
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
