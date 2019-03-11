package pack

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/buildpack/pack/logging"

	"github.com/buildpack/lifecycle"
	lcimg "github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/config"
)

type RebaseConfig struct {
	Image        lcimg.Image
	NewBaseImage lcimg.Image
}

type RebaseFactory struct {
	Logger  *logging.Logger
	Config  *config.Config
	Fetcher Fetcher
}

type RebaseFlags struct {
	RepoName string
	Publish  bool
	NoPull   bool
	RunImage string
}

func (f *RebaseFactory) RebaseConfigFromFlags(ctx context.Context, flags RebaseFlags, stdout io.Writer) (RebaseConfig, error) {
	var newImageFn func(string) (lcimg.Image, error)
	if flags.Publish {
		newImageFn = f.Fetcher.FetchRemoteImage
	} else {
		newImageFn = func(name string) (lcimg.Image, error) {
			if !flags.NoPull {
				return f.Fetcher.FetchUpdatedLocalImage(ctx, name, stdout)

			} else {
				return f.Fetcher.FetchLocalImage(name)

			}
		}
	}

	appImage, err := newImageFn(flags.RepoName)
	if err != nil {
		return RebaseConfig{}, err
	}

	var runImageName string
	if flags.RunImage != "" {
		runImageName = flags.RunImage
	} else {
		runImageName, err = appImage.Label(RunImageLabel) // TODO : const the label name
		if err != nil {
			return RebaseConfig{}, err
		}
	}

	if runImageName == "" {
		return RebaseConfig{}, errors.New("run image must be specified")
	}

	baseImage, err := newImageFn(runImageName)
	if err != nil {
		return RebaseConfig{}, err
	}

	return RebaseConfig{
		Image:        appImage,
		NewBaseImage: baseImage,
	}, nil
}

func (f *RebaseFactory) Rebase(cfg RebaseConfig) error {
	label, err := cfg.Image.Label("io.buildpacks.lifecycle.metadata")
	if err != nil {
		return err
	}
	var metadata lifecycle.AppImageMetadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return err
	}
	if err := cfg.Image.Rebase(metadata.RunImage.TopLayer, cfg.NewBaseImage); err != nil {
		return err
	}

	metadata.RunImage.SHA, err = cfg.NewBaseImage.Digest()
	if err != nil {
		return err
	}
	metadata.RunImage.TopLayer, err = cfg.NewBaseImage.TopLayer()
	if err != nil {
		return err
	}
	newLabel, err := json.Marshal(metadata)
	if err := cfg.Image.SetLabel("io.buildpacks.lifecycle.metadata", string(newLabel)); err != nil {
		return err
	}

	_, err = cfg.Image.Save()
	if err != nil {
		return err
	}
	return nil
}
