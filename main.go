package main

import (
	"chatgpt-adapter/wire"
	"github.com/iocgo/sdk"
	"github.com/iocgo/sdk/errors"
	"os"
)

func main() {
	ensureConfigFile()

	ctx := errors.New(nil)
	{
		if err := errors.Try1(ctx, func() (c *sdk.Container, err error) {
			c = sdk.NewContainer()
			err = wire.Injects(c)
			return
		}).Run(); err != nil {
			panic(err)
		}
	}
}

func ensureConfigFile() {
	if _, err := os.Stat("config.yaml"); err == nil {
		return
	} else if !os.IsNotExist(err) {
		panic(err)
	}

	content := "server:\n  port: ${PORT}\n  password: ${PASSWORD}\n"
	if err := os.WriteFile("config.yaml", []byte(content), 0644); err != nil {
		panic(err)
	}
}
