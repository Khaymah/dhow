package keys

import "os"

var DekAADForTest = dekAAD

func AppendKeyringVersionWith(path string, writeAt func(*os.File, []byte, int64) (int, error)) (int32, error) {
	return appendKeyringVersion(path, FileRules{}, writeAt)
}
