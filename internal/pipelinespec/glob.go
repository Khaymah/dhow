package pipelinespec

import "github.com/Khaymah/dhow/internal/pathglob"

func ValidPattern(p string) bool { return pathglob.ValidPattern(p) }

func MatchPattern(pattern, name string) bool { return pathglob.MatchPattern(pattern, name) }
