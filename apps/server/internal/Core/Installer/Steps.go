package installer

import (
	"io/fs"

	engine "github.com/Khaymah/dhow/internal/installer"
)

type (
	Step     = engine.Step
	Plan     = engine.Plan
	Runner   = engine.Runner
	fileSpec = engine.File
)

func sameFile(sys System, path string, data []byte, perm fs.FileMode) (bool, error) {
	return engine.SameFile(sys, path, data, perm)
}

func FileStep(sys System, title, path string, data []byte, mode FileMode) Step {
	return engine.FileStep(sys, title, path, data, mode)
}

func SecretFileStep(sys System, title, path string, generate func() ([]byte, error)) Step {
	return engine.SecretFileStep(sys, title, path, modeRootPrivate, generate)
}

func ContainerSecretFileStep(sys System, title, path string, generate func() ([]byte, error)) Step {
	return engine.SecretFileStep(sys, title, path, modeContainer, generate)
}

func DirStep(sys System, title, path string, mode FileMode) Step {
	return engine.DirStep(sys, title, path, mode)
}

func CommandStep(sys System, title string, done *Command, apply []Command, undo []Command) Step {
	return engine.CommandStep(sys, title, done, apply, undo)
}
