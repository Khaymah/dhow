package podman

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const userNSSize = 65536

type idRange struct {
	inside  int64
	outside int64
	count   int64
}

func (r idRange) holds(id int64) bool { return id >= r.outside && id < r.outside+r.count }

func (r idRange) overlaps(o idRange) bool {
	return r.outside < o.outside+o.count && o.outside < r.outside+r.count
}

func readIDMap(path string) ([]idRange, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // The uid_map or gid_map of the sandbox's first process, under /proc.
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseIDMap(string(raw))
}

func parseIDMap(raw string) ([]idRange, error) {
	var out []idRange
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			return nil, fmt.Errorf("%w: an id map line has %d fields", ErrPolicyMismatch, len(f))
		}
		var nums [3]int64
		for i, v := range f {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: an id map value does not parse", ErrPolicyMismatch)
			}
			nums[i] = n
		}
		out = append(out, idRange{inside: nums[0], outside: nums[1], count: nums[2]})
	}
	return out, nil
}

func verifyIDMap(ranges []idRange, hostID int64, others [][]idRange) error {
	var total int64
	for _, r := range ranges {
		if r.holds(hostID) || r.holds(0) {
			return fmt.Errorf("%w: the user namespace maps the runner's own id or host root", ErrPolicyMismatch)
		}
		for _, other := range others {
			for _, o := range other {
				if r.overlaps(o) {
					return fmt.Errorf("%w: the user namespace overlaps another live sandbox", ErrPolicyMismatch)
				}
			}
		}
		total += r.count
	}
	if len(ranges) == 0 || total > userNSSize {
		return fmt.Errorf("%w: the user namespace maps %d ids, not at most %d", ErrPolicyMismatch, total, userNSSize)
	}
	return nil
}

func exactMap(ranges []idRange, hostStart int64) error {
	if len(ranges) != 1 || ranges[0] != (idRange{inside: 0, outside: hostStart, count: userNSSize}) {
		return fmt.Errorf("%w: the user namespace is not the attempt's id range starting at %d", ErrPolicyMismatch, hostStart)
	}
	return nil
}

func (b *box) verifyUserNS(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("%w: the sandbox has no running process", ErrPolicyMismatch)
	}
	uids, err := readIDMap(fmt.Sprintf("/proc/%d/uid_map", pid))
	if err != nil {
		return err
	}
	if err := verifyIDMap(uids, int64(os.Getuid()), b.d.liveRanges(b.name)); err != nil {
		return fmt.Errorf("uid_map: %w", err)
	}
	gids, err := readIDMap(fmt.Sprintf("/proc/%d/gid_map", pid))
	if err != nil {
		return err
	}
	if err := verifyIDMap(gids, int64(os.Getgid()), nil); err != nil {
		return fmt.Errorf("gid_map: %w", err)
	}
	if err := exactMap(uids, b.idStart); err != nil {
		return fmt.Errorf("uid_map: %w", err)
	}
	if err := exactMap(gids, b.idStart); err != nil {
		return fmt.Errorf("gid_map: %w", err)
	}
	b.d.track(b.name, uids)
	return nil
}

func (d *Driver) liveRanges(except string) [][]idRange {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([][]idRange, 0, len(d.live))
	for name, ranges := range d.live {
		if name != except {
			out = append(out, ranges)
		}
	}
	return out
}

func (d *Driver) track(name string, ranges []idRange) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.live[name] = ranges
}

func (d *Driver) untrack(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.live, name)
}
