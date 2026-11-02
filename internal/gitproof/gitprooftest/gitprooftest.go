package gitprooftest

import (
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/Khaymah/dhow/internal/gitproof"
)

type Commit struct {
	Sha     string
	Objects []gitproof.Object
}

func Tree(entries ...gitproof.Entry) []byte {
	var b []byte
	for _, e := range entries {
		id, err := hex.DecodeString(e.ID)
		if err != nil {
			return nil
		}
		b = append(b, e.Mode+" "+e.Name+"\x00"...)
		b = append(b, id...)
	}
	return b
}

func CommitBody(tree, message string) []byte {
	return []byte("tree " + tree + "\nauthor Dhow <dhow@example.com> 1700000000 +0000\ncommitter Dhow <dhow@example.com> 1700000000 +0000\n\n" + message + "\n")
}

func File(f gitproof.Format, path, source string) Commit {
	return WithEntry(f, path, gitproof.ModeFile, []byte(source))
}

func WithEntry(f gitproof.Format, path, mode string, data []byte) Commit {
	segments := strings.Split(path, "/")
	blob := gitproof.Object{Kind: gitproof.KindBlob, Data: data}
	id, entryMode := f.ID(gitproof.KindBlob, data), mode
	trees := make([]gitproof.Object, len(segments))
	for i := len(segments) - 1; i >= 0; i-- {
		body := Tree(gitproof.Entry{Mode: entryMode, Name: segments[i], ID: id})
		trees[i] = gitproof.Object{Kind: gitproof.KindTree, Data: body}
		id, entryMode = f.ID(gitproof.KindTree, body), gitproof.ModeTree
	}
	commit := CommitBody(id, "add "+path)
	objects := append([]gitproof.Object{{Kind: gitproof.KindCommit, Data: commit}}, trees...)
	if mode == gitproof.ModeFile || mode == gitproof.ModeExecutable {
		objects = append(objects, blob)
	}
	return Commit{Sha: f.ID(gitproof.KindCommit, commit), Objects: objects}
}

func Empty(f gitproof.Format) Commit {
	commit := CommitBody(f.ID(gitproof.KindTree, nil), "empty")
	return Commit{
		Sha:     f.ID(gitproof.KindCommit, commit),
		Objects: []gitproof.Object{{Kind: gitproof.KindCommit, Data: commit}, {Kind: gitproof.KindTree, Data: []byte{}}},
	}
}

func Log(objects []gitproof.Object) string {
	var b strings.Builder
	for _, o := range objects {
		b.WriteString(gitproof.ObjectPrefix + o.Kind + "\n")
		encoded := base64.StdEncoding.EncodeToString(o.Data)
		for len(encoded) > 0 {
			n := min(len(encoded), 76)
			b.WriteString(gitproof.DataPrefix + encoded[:n] + "\n")
			encoded = encoded[n:]
		}
	}
	return b.String()
}
