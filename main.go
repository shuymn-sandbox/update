package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

type Command struct {
	Name string
	Args []string
}

func (c *Command) available() bool {
	_, err := exec.LookPath(c.Name)
	return err == nil
}

func (c *Command) print(rd io.Reader, prefix string) error {
	r := bufio.NewReader(rd)
	logger := log.New(os.Stdout, prefix, log.Lmsgprefix)
	for {
		row, err := r.ReadString('\n')
		if len(row) > 0 {
			logger.Print(row)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (c *Command) copy(rd io.Reader) (string, error) {
	r := bufio.NewReader(rd)
	var b strings.Builder
	for {
		row, err := r.ReadString('\n')
		if len(row) > 0 {
			b.WriteString(row)
		}
		if err != nil {
			if err == io.EOF {
				return b.String(), nil
			}
			return b.String(), err
		}
	}
}

func (c *Command) execute() error {
	if !c.available() {
		return nil
	}

	cmd := exec.Command(c.Name, c.Args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	prefix := "[" + c.Name + "] "
	var eg errgroup.Group

	eg.Go(func() error {
		return c.print(stdout, prefix)
	})

	eg.Go(func() error {
		str, err := c.copy(stderr)
		if err != nil {
			return err
		}
		if str != "" {
			return errors.New(str)
		}
		return nil
	})

	if err = eg.Wait(); err != nil {
		return err
	}

	if err = cmd.Wait(); err != nil {
		return err
	}

	return nil
}

type ExecutionError struct {
	Name  string
	Error error
}

func main() {
	cmds := []Command{
		{Name: "brew", Args: []string{"upgrade"}},
		{Name: "anyenv", Args: []string{"update"}},
		{Name: "anyenv", Args: []string{"git", "pull"}},
		{Name: "stack", Args: []string{"upgrade"}},
		{Name: "npm", Args: []string{"i", "-g", "npm"}},
		{Name: "rustup", Args: []string{"self", "update"}},
	}

	errChan := make(chan ExecutionError, len(cmds))
	var wg sync.WaitGroup

	for _, cmd := range cmds {
		wg.Add(1)
		cmd := cmd
		go func() {
			defer wg.Done()
			if err := cmd.execute(); err != nil {
				errChan <- ExecutionError{
					Name:  cmd.Name,
					Error: err,
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	errors := make([]ExecutionError, 0, len(cmds))
	for err := range errChan {
		errors = append(errors, err)
	}

	code := 0
	if len(errors) > 0 {
		logger := log.New(os.Stderr, "", log.Lmsgprefix)
		for _, err := range errors {
			fmt.Print("\n")
			logger.SetPrefix("[" + err.Name + "] ")
			s := bufio.NewScanner(strings.NewReader(err.Error.Error()))
			for s.Scan() {
				logger.Print(s.Text())
			}

			if s.Err() != nil {
				fmt.Printf("Scanner error: %q\n", s.Err())
			}
		}
		code = 1
	}

	os.Exit(code)
}
