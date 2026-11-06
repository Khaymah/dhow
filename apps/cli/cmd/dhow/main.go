package main

import (
	"os"

	app "github.com/Khaymah/dhow/apps/cli/internal/App"
)

func main() {
	ctx, stop := app.SignalContext()
	code := app.Main(os.Args[1:], app.Env{Getenv: os.Getenv, In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Ctx: ctx})
	stop()
	os.Exit(code)
}
