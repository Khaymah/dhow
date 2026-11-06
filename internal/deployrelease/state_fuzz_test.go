package deployrelease

import "testing"

func FuzzStateFile(f *testing.F) {
	f.Add([]byte(`{"State":"idle","Release":"10-aaaaaaaa-0","Previous":"9-aaaaaaaa-0","At":"2026-11-05T12:00:00Z"}`))
	f.Add([]byte(`{"State":"switching","Release":"10-aaaaaaaa-0","Previous":"","At":"2026-11-05T12:00:00Z"}`))
	f.Add([]byte(`{"State":"flying","Release":"x"}`))
	f.Add([]byte(`{"State":"idle","Release":"../../etc","Previous":""}`))
	f.Add([]byte(`{"State":"idle","Release":"a"} {"State":"idle","Release":"b"}`))
	f.Add([]byte(`{"State":"idle","Release":"a","Extra":1}`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, raw []byte) {
		s, ok := validState(raw)
		if !ok {
			return
		}
		if !validID(s.Release) || (s.Previous != "" && !validID(s.Previous)) {
			t.Fatalf("accepted a state file with a release that is not a release id: %q", raw)
		}
		switch s.State {
		case "idle", "switching", "rolling_back":
		default:
			t.Fatalf("accepted the state %q", s.State)
		}
	})
}
