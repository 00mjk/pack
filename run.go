package pack

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/buildpack/pack/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

type RunFlags struct {
	AppDir   string
	Builder  string
	RunImage string
	Port     string
}

type RunConfig struct {
	Port  string
	Build Task
	// All below are from BuildConfig
	AppDir   string
	Builder  string
	RunImage string
	RepoName string
	// Above are copied from BuildFlags are set by init
	Cli    Docker
	Stdout io.Writer
	Stderr io.Writer
	Log    *log.Logger
	FS     FS
	Config *config.Config
	Images Images
	// Above are copied from BuildFactory
	WorkspaceVolume string
	CacheVolume     string
}

func (bf *BuildFactory) RunConfigFromFlags(f *RunFlags) (*RunConfig, error) {
	bc, err := bf.BuildConfigFromFlags(&BuildFlags{
		AppDir:   f.AppDir,
		Builder:  f.Builder,
		RunImage: f.RunImage,
		RepoName: f.repoName(),
		Publish:  false,
		NoPull:   false,
	})
	if err != nil {
		return nil, err
	}
	rc := &RunConfig{
		Build: bc,
		Port:  f.Port,
		// All below are from BuildConfig
		AppDir:   bc.AppDir,
		Builder:  bc.Builder,
		RunImage: bc.RunImage,
		RepoName: bc.RepoName,
		// Above are copied from BuildFlags are set by init
		Cli:    bc.Cli,
		Stdout: bc.Stdout,
		Stderr: bc.Stderr,
		Log:    bc.Log,
		FS:     bc.FS,
		Config: bc.Config,
		Images: bc.Images,
		// Above are copied from BuildFactory
		WorkspaceVolume: bc.WorkspaceVolume,
		CacheVolume:     bc.CacheVolume,
	}

	return rc, nil
}

func Run(appDir, buildImage, runImage, port string, makeStopCh func() <-chan struct{}) error {
	bf, err := DefaultBuildFactory()
	if err != nil {
		return err
	}
	r, err := bf.RunConfigFromFlags(&RunFlags{
		AppDir:   appDir,
		Builder:  buildImage,
		RunImage: runImage,
		Port:     port,
	})
	if err != nil {
		return err
	}
	return r.Run(makeStopCh)
}

func (r *RunConfig) Run(makeStopCh func() <-chan struct{}) error {
	ctx := context.Background()

	err := r.Build.Run()
	if err != nil {
		return err
	}

	fmt.Println("*** RUNNING:")
	exposedPorts, portBindings, err := parsePorts(r.Port)
	if err != nil {
		return err
	}
	ctr, err := r.Cli.ContainerCreate(ctx, &container.Config{
		Image:        r.RepoName,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: exposedPorts,
	}, &container.HostConfig{
		AutoRemove:   true,
		PortBindings: portBindings,
	}, nil, "")

	logContainerListening(r.Log, portBindings)
	running := true
	stopCh := makeStopCh()
	go func() {
		<-stopCh
		running = false
		r.Cli.ContainerRemove(ctx, ctr.ID, types.ContainerRemoveOptions{
			Force: true,
		})
	}()
	if err = r.Cli.RunContainer(ctx, ctr.ID, r.Stdout, r.Stderr); err != nil && running {
		return errors.Wrap(err, "run container")
	}

	return nil
}

func (r *RunFlags) repoName() string {
	dir, _ := filepath.Abs(r.AppDir)
	// we can ignore errors here because they will be caught later by the Build command
	h := md5.New()
	io.WriteString(h, dir)
	return fmt.Sprintf("pack.local/run/%x", h.Sum(nil))
}

func parsePorts(port string) (nat.PortSet, nat.PortMap, error) {
	ports := strings.Split(port, ",")
	for i, p := range ports {
		p = strings.TrimSpace(p)
		if _, err := strconv.Atoi(p); err == nil {
			// default simple port to localhost and 8080 inside the container
			p = fmt.Sprintf("127.0.0.1:%s:8080/tcp", p)
		}
		ports[i] = p
	}

	return nat.ParsePortSpecs(ports)
}

func logContainerListening(log *log.Logger, portBindings nat.PortMap) {
	// TODO handle case with multiple ports, for now when there is more than
	// one port we assume you know what you're doing and don't need guidance
	if len(portBindings) == 1 {
		for _, bindings := range portBindings {
			if len(bindings) == 1 {
				binding := bindings[0]
				host := binding.HostIP
				port := binding.HostPort
				if host == "127.0.0.1" {
					host = "localhost"
				}
				log.Printf("Starting container listening at http://%s:%s/\n", host, port)
			}
		}
	}
}
