package downloader_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/api"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildpack/downloader"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/config"
	image "github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpackDownloader(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "BuildpackDownloader", testBuildpackDownloader, spec.Report(report.Terminal{}))
}

func testBuildpackDownloader(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController       *gomock.Controller
		mockDownloader       *testmocks.MockDownloader
		mockImageFactory     *testmocks.MockImageFactory
		mockImageFetcher     *testmocks.MockImageFetcher
		mockRegistryResolver *testmocks.MockRegistryResolver
		mockDockerClient     *testmocks.MockCommonAPIClient
		buildpackDownloader  client.BuildpackDownloader
		logger               logging.Logger
		out                  bytes.Buffer
		tmpDir               string
	)

	var createBuildpack = func(descriptor dist.BuildpackDescriptor) string {
		bp, err := ifakes.NewFakeBuildpackBlob(descriptor, 0644)
		h.AssertNil(t, err)
		url := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
		mockDownloader.EXPECT().Download(gomock.Any(), url).Return(bp, nil).AnyTimes()
		return url
	}

	var createPackage = func(imageName string) *fakes.Image {
		packageImage := fakes.NewImage(imageName, "", nil)
		mockImageFactory.EXPECT().NewImage(packageImage.Name(), false, "linux").Return(packageImage, nil)

		pack, err := client.NewClient(
			client.WithLogger(logger),
			client.WithDownloader(mockDownloader),
			client.WithImageFactory(mockImageFactory),
			client.WithFetcher(mockImageFetcher),
			client.WithDockerClient(mockDockerClient),
		)
		h.AssertNil(t, err)

		h.AssertNil(t, pack.PackageBuildpack(context.TODO(), client.PackageBuildpackOptions{
			Name: packageImage.Name(),
			Config: pubbldpkg.Config{
				Platform: dist.Platform{OS: "linux"},
				Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
					API:    api.MustParse("0.3"),
					Info:   dist.BuildpackInfo{ID: "example/foo", Version: "1.1.0"},
					Stacks: []dist.Stack{{ID: "some.stack.id"}},
				})},
			},
			Publish: true,
		}))

		return packageImage
	}

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&out, &out, ilogging.WithVerbose())
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockRegistryResolver = testmocks.NewMockRegistryResolver(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)
		mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)
		mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
		mockDownloader.EXPECT().Download(gomock.Any(), "some/buildpack/dir").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()

		buildpackDownloader = downloader.NewBuildpackDownloader(logger, mockImageFetcher, mockDownloader, mockRegistryResolver)

		mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

		mockRegistryResolver.EXPECT().
			Resolve("some-registry", "urn:cnb:registry:example/foo@1.1.0").
			Return("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", nil).
			AnyTimes()
		mockRegistryResolver.EXPECT().
			Resolve("some-registry", "example/foo@1.1.0").
			Return("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", nil).
			AnyTimes()

		var err error
		tmpDir, err = ioutil.TempDir("", "buildpack-downloader-test")
		h.AssertNil(t, err)

		// registryFixture := h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))

		// packHome := filepath.Join(tmpDir, ".pack")
		// configPath := filepath.Join(packHome, "config.toml")

		// h.AssertNil(t, cfg.Write(cfg.Config{
		// 	Registries: []cfg.Registry{
		// 		{
		// 			Name: "some-registry",
		// 			Type: "github",
		// 			URL:  registryFixture,
		// 		},
		// 	},
		// }, configPath))
		// h.AssertNil(t, err)
	})

	it.After(func() {
		mockController.Finish()
		h.AssertNil(t, os.RemoveAll(tmpDir))
		// os.Unsetenv("PACK_HOME")
	})

	when("#DownloadBuildpack", func() {
		var (
			packageImage             *fakes.Image
			buildpackDownloadOptions = downloader.BuildpackDownloadOptions{ImageOS: "linux"}
		)

		shouldFetchPackageImageWith := func(demon bool, pull config.PullPolicy) {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), image.FetchOptions{Daemon: demon, PullPolicy: pull}).Return(packageImage, nil)
		}

		when("package image lives in cnb registry", func() {
			it.Before(func() {
				packageImage = createPackage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7")
			})

			when("daemon=true and pull-policy=always", func() {
				it("should pull and use local package image", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						RegistryName: "some-registry",
						ImageOS:      "linux",
						Daemon:       true,
						PullPolicy:   config.PullAlways,
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "urn:cnb:registry:example/foo@1.1.0", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("ambigious URI provided", func() {
				it("should find package in registry", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						RegistryName: "some-registry",
						ImageOS:      "linux",
						Daemon:       true,
						PullPolicy:   config.PullAlways,
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "example/foo@1.1.0", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})
		})

		when("package image lives in docker registry", func() {
			it.Before(func() {
				packageImage = createPackage("docker.io/some/package-" + h.RandString(12))
			})

			prepareFetcherWithMissingPackageImage := func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), gomock.Any()).Return(nil, image.ErrNotFound)
			}

			when("image key is provided", func() {
				it("should succeed", func() {
					packageImage = createPackage("some/package:tag")
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						Daemon:     true,
						PullPolicy: config.PullAlways,
						ImageOS:    "linux",
						ImageName:  "some/package:tag",
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("daemon=true and pull-policy=always", func() {
				it("should pull and use local package image", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     true,
						PullPolicy: config.PullAlways,
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("daemon=false and pull-policy=always", func() {
				it("should use remote package image", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     false,
						PullPolicy: config.PullAlways,
					}

					shouldFetchPackageImageWith(false, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("daemon=false and pull-policy=always", func() {
				it("should use remote package URI", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						ImageOS:    "linux",
						Daemon:     false,
						PullPolicy: config.PullAlways,
					}
					shouldFetchPackageImageWith(false, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), packageImage.Name(), buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("publish=true and pull-policy=never", func() {
				it("should push to registry and not pull package image", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     false,
						PullPolicy: config.PullNever,
					}

					shouldFetchPackageImageWith(false, config.PullNever)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("daemon=true pull-policy=never and there is no local package image", func() {
				it("should fail without trying to retrieve package image from registry", func() {
					buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     true,
						PullPolicy: config.PullNever,
					}
					prepareFetcherWithMissingPackageImage()
					_, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertError(t, err, "not found")
				})
			})
		})
		when("package lives on filesystem", func() {
			it("should successfully retrieve package from absolute path", func() {
				buildpackPath := filepath.Join("testdata", "buildpack")
				buildpackURI, _ := paths.FilePathToURI(buildpackPath, "")
				mockDownloader.EXPECT().Download(gomock.Any(), buildpackURI).Return(blob.NewBlob(buildpackPath), nil).AnyTimes()
				mainBP, _, err := buildpackDownloader.Download(context.TODO(), buildpackURI, buildpackDownloadOptions)
				h.AssertNil(t, err)
				h.AssertEq(t, mainBP.Descriptor().Info.ID, "bp.one")
			})
			it("should successfully retrieve package from relative path", func() {
				buildpackPath := filepath.Join("testdata", "buildpack")
				buildpackURI, _ := paths.FilePathToURI(buildpackPath, "")
				mockDownloader.EXPECT().Download(gomock.Any(), buildpackURI).Return(blob.NewBlob(buildpackPath), nil).AnyTimes()
				buildpackDownloadOptions = downloader.BuildpackDownloadOptions{
					ImageOS:         "linux",
					RelativeBaseDir: "testdata",
				}
				mainBP, _, err := buildpackDownloader.Download(context.TODO(), "buildpack", buildpackDownloadOptions)
				h.AssertNil(t, err)
				h.AssertEq(t, mainBP.Descriptor().Info.ID, "bp.one")
			})
		})
		when("package image is not a valid package", func() {
			it("should error", func() {
				notPackageImage := fakes.NewImage("docker.io/not/package", "", nil)

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), gomock.Any()).Return(notPackageImage, nil)
				h.AssertNil(t, notPackageImage.SetLabel("io.buildpacks.buildpack.layers", ""))

				buildpackDownloadOptions.ImageName = notPackageImage.Name()
				_, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
				h.AssertError(t, err, "extracting buildpacks from 'docker.io/not/package': could not find label 'io.buildpacks.buildpackage.metadata'")
			})
		})
		when("invalid buildpack URI", func() {
			when("buildpack URI is from=builder:fake", func() {
				it("errors", func() {
					_, _, err := buildpackDownloader.Download(context.TODO(), "from=builder:fake", buildpackDownloadOptions)
					h.AssertError(t, err, "'from=builder:fake' is not a valid identifier")
				})
			})

			when("buildpack URI is from=builder", func() {
				it("errors", func() {
					_, _, err := buildpackDownloader.Download(context.TODO(), "from=builder", buildpackDownloadOptions)
					h.AssertError(t, err,
						"invalid locator: FromBuilderLocator")
				})
			})

			when("can't resolve buildpack in registry", func() {
				it("errors", func() {
					mockRegistryResolver.EXPECT().
						Resolve("://bad-url", "urn:cnb:registry:fake").
						Return("", errors.New("bad mhkay")).
						AnyTimes()

					buildpackDownloadOptions.RegistryName = "://bad-url"
					_, _, err := buildpackDownloader.Download(context.TODO(), "urn:cnb:registry:fake", buildpackDownloadOptions)
					h.AssertError(t, err, "locating in registry")
				})
			})

			when("can't download image from registry", func() {
				it("errors", func() {
					packageImage := fakes.NewImage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", "", nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), image.FetchOptions{Daemon: false, PullPolicy: config.PullAlways}).Return(nil, errors.New("failed to pull"))

					buildpackDownloadOptions.RegistryName = "some-registry"
					_, _, err := buildpackDownloader.Download(context.TODO(), "urn:cnb:registry:example/foo@1.1.0", buildpackDownloadOptions)
					h.AssertError(t, err,
						"extracting from registry")
				})
			})
			when("buildpack URI is an invalid locator", func() {
				it("errors", func() {
					_, _, err := buildpackDownloader.Download(context.TODO(), "nonsense string here", buildpackDownloadOptions)
					h.AssertError(t, err,
						"invalid locator: InvalidLocator")
				})
			})
		})
	})
}
