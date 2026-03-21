package lifecycle

import (
	"bytes"
	"errors"
	"io"
	"os"
)

type lifecycleWorkspace struct {
	root string
	home string
	work string
}

func captureStderrOutput(run func() error) (string, error) {
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stderr = w
	defer func() {
		os.Stderr = orig
	}()

	runErr := run()
	closeErr := w.Close()

	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	readCloseErr := r.Close()

	return buf.String(), errors.Join(runErr, closeErr, copyErr, readCloseErr)
}
