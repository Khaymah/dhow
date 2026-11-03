package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultConfigs = "2g:2g,512m:2g,384m:2g,256m:2g,192m:2g,128m:2g,2g:512m,2g:384m,2g:256m,2g:192m,2g:128m,256m:256m,384m:384m"
	ownerEmail     = "owner@bench.test"
	prepWorkers    = 4
)

type options struct {
	root, image, dbImage, project, configs, runner, out, work, baseline, socket, cpus string
	port                                                                              int
	idle, light, heavy                                                                time.Duration
	lightUsers, heavyUsers, lightJobs, heavyJobs                                      int
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.StringVar(&o.root, "root", ".", "the repository root")
	fs.StringVar(&o.image, "image", "", "the Dhow server image to measure")
	fs.StringVar(&o.dbImage, "postgres-image", "", "the PostgreSQL 18 image, when not the one pinned in deploy/compose")
	fs.StringVar(&o.project, "project", "bench-dhow", "the Compose project name")
	fs.StringVar(&o.configs, "configs", defaultConfigs, "memory limits to try, as server:postgres pairs; the first one is the reference")
	fs.StringVar(&o.runner, "runner", "", "a dhow-runner built with -tags dhowtest, to run jobs with the fake driver; empty leaves jobs queued")
	fs.StringVar(&o.out, "out", "bin/bench/results.json", "where to write the results")
	fs.StringVar(&o.work, "work", "bin/bench/work", "a work directory for the runner identities")
	fs.StringVar(&o.baseline, "baseline", "", "fail when a peak is over this baseline file by more than its tolerance")
	fs.StringVar(&o.socket, "docker-socket", "/var/run/docker.sock", "the Docker Engine socket")
	fs.StringVar(&o.cpus, "cpus", "2.0", "the CPU limit of each container")
	fs.IntVar(&o.port, "port", 56080, "the host port for the server")
	fs.DurationVar(&o.idle, "idle", 30*time.Second, "how long to measure the idle phase")
	fs.DurationVar(&o.light, "light", 60*time.Second, "how long to run the light load")
	fs.DurationVar(&o.heavy, "heavy", 90*time.Second, "how long to run the heavy load")
	fs.IntVar(&o.lightUsers, "light-users", 10, "users browsing in the light phase")
	fs.IntVar(&o.heavyUsers, "heavy-users", 50, "users browsing in the heavy phase")
	fs.IntVar(&o.lightJobs, "light-jobs", 3, "concurrent job operations in the light phase")
	fs.IntVar(&o.heavyJobs, "heavy-jobs", 20, "concurrent job operations in the heavy phase")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if o.image == "" {
		return o, errors.New("-image is required")
	}
	return o, nil
}

func parseConfigs(raw string) ([]Limits, error) {
	var out []Limits
	for _, part := range strings.Split(raw, ",") {
		server, db, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || server == "" || db == "" {
			return nil, fmt.Errorf("bad limits %q: want server:postgres, such as 512m:1g", part)
		}
		out = append(out, Limits{Server: server, DB: db})
	}
	return out, nil
}

func (o options) phases() []Phase {
	return []Phase{
		{Name: "idle", Duration: o.idle, Runners: []int{1}},
		{Name: "light", Duration: o.light, Users: o.lightUsers, Think: time.Second, JobWorkers: o.lightJobs, Runners: []int{o.lightJobs}},
		{Name: "heavy", Duration: o.heavy, Users: o.heavyUsers, Think: 500 * time.Millisecond, JobWorkers: o.heavyJobs, Runners: split(o.heavyJobs)},
	}
}

func split(jobs int) []int {
	const perRunner = 10
	var out []int
	for jobs > 0 {
		n := min(jobs, perRunner)
		out = append(out, n)
		jobs -= n
	}
	if len(out) == 0 {
		out = []int{1}
	}
	return out
}

func prepare(ctx context.Context, w *World, members, admins, runners int) error {
	token, err := w.Stack.SetupToken(ctx)
	if err != nil {
		return err
	}
	password, err := randomHex(16)
	if err != nil {
		return err
	}
	if w.Org, err = w.API.Setup(ctx, token, ownerEmail, password); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if w.Owner, err = w.API.SignIn(ctx, ownerEmail, password, "198.19.0.1"); err != nil {
		return fmt.Errorf("owner sign-in: %w", err)
	}
	w.Password = password
	w.Members = make([]string, members)
	errs := make([]error, members)
	var wg sync.WaitGroup
	slots := make(chan struct{}, prepWorkers)
	for i := range members {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			email := "member" + strconv.Itoa(i) + "@bench.test"
			role := "developer"
			if i < admins {
				role = "admin"
			}
			w.Members[i] = email
			errs[i] = w.API.AddMember(ctx, &w.Owner, w.Org, email, role, password)
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("members: %w", err)
	}
	return w.Runners.Register(ctx, w.API, &w.Owner, w.Org, runners)
}

func runConfig(ctx context.Context, o options, limits Limits, log *slog.Logger) ConfigResult {
	res := ConfigResult{Limits: limits}
	st := &Stack{
		Root: o.root, Project: o.project, Image: o.image, DBImage: o.dbImage, Port: o.port, CPUs: o.cpus,
		Limits: limits, Socket: o.socket, Work: o.work,
	}
	defer st.Down(context.WithoutCancel(ctx))
	runners := &Runners{Binary: o.runner, Dir: o.work, URL: st.BaseURL()}
	defer runners.Stop()
	log.Info("starting the stack", "limits", limits.String())
	if err := st.Up(ctx); err != nil {
		res.Problem = "start: " + err.Error()
		return res
	}
	phases := o.phases()
	maxRunners := 0
	for _, p := range phases {
		maxRunners = max(maxRunners, len(p.Runners))
	}
	hc := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 256}}
	w := &World{API: &API{Base: st.BaseURL(), HC: hc}, Runners: runners, Stack: st}
	w.Admins = max(o.heavyJobs, o.lightJobs, 1)
	if err := prepare(ctx, w, w.Admins+max(o.heavyUsers, o.lightUsers, 1), w.Admins, maxRunners); err != nil {
		health, _ := st.Health(ctx)
		res.Problem = strings.TrimSuffix("prepare: "+err.Error()+"; "+health, "; ")
		return res
	}
	for _, p := range phases {
		if !runners.Enabled() {
			p.Runners = nil
		}
		log.Info("running a phase", "limits", limits.String(), "phase", p.Name, "for", p.Duration)
		r := w.Run(ctx, p)
		log.Info("phase done", "phase", p.Name, "ok", r.OK, "server_peak_mib", f1(r.Server.PeakMiB), "db_peak_mib", f1(r.DB.PeakMiB), "problem", r.Problem)
		res.Phases = append(res.Phases, r)
		if !r.OK {
			break
		}
	}
	return res
}

func run(ctx context.Context, args []string) error {
	o, err := parseFlags(args)
	if err != nil {
		return err
	}
	limits, err := parseConfigs(o.configs)
	if err != nil {
		return err
	}
	if o.work, err = filepath.Abs(o.work); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rep := Report{Date: today(), ServerImage: o.image, PostgresImage: o.dbImage, HostCPUs: runtime.NumCPU(), CPUsPerContainer: o.cpus}
	if rep.ServerImageBytes, err = ImageBytes(ctx, o.image); err != nil {
		return err
	}
	if o.dbImage != "" {
		if rep.PostgresImageBytes, err = ImageBytes(ctx, o.dbImage); err != nil {
			return err
		}
	}
	for _, l := range limits {
		rep.Configs = append(rep.Configs, runConfig(ctx, o, l, log))
		if ctx.Err() != nil {
			break
		}
		if err := os.MkdirAll(filepath.Dir(o.out), 0o750); err != nil {
			return err
		}
		if err := rep.Write(o.out); err != nil {
			return err
		}
	}
	_, _ = os.Stdout.WriteString(rep.Table())
	if o.baseline == "" {
		return ctx.Err()
	}
	problems, err := rep.Compare(o.baseline)
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return errors.New("over the published numbers:\n  " + strings.Join(problems, "\n  "))
	}
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		_, _ = os.Stderr.WriteString("bench: " + err.Error() + "\n")
		os.Exit(1)
	}
}
