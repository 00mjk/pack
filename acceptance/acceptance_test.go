package acceptance_test

import (
	"encoding/hex"
	"errors"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var pack string

func TestPack(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	pack = os.Getenv("PACK_PATH")
	if pack == "" {
		packTmpDir, err := ioutil.TempDir("", "pack.acceptance.binary.")
		if err != nil {
			panic(err)
		}
		if txt, err := exec.Command("go", "build", "-o", filepath.Join(packTmpDir, "pack"), "../cmd/pack").CombinedOutput(); err != nil {
			fmt.Println(string(txt))
			panic(err)
		}
		pack = filepath.Join(packTmpDir, "pack")
		defer os.RemoveAll(packTmpDir)
	}

	hostMachineIP, err := findHostMachineIP()
	if err != nil {
		panic(err)
	}
	os.Setenv("PACK_HOST_MACHINE_IP", hostMachineIP)

	spec.Run(t, "pack", testPack, spec.Report(report.Terminal{}))
}

func testPack(t *testing.T, when spec.G, it spec.S) {
	var homeDir string

	it.Before(func() {
		if _, err := os.Stat(pack); os.IsNotExist(err) {
			t.Fatal("No file found at PACK_PATH environment variable:", pack)
		}

		var err error
		homeDir, err = ioutil.TempDir("", "buildpack.pack.build.homedir.")
		if err != nil {
			t.Fatal(err)
		}
	})
	it.After(func() {
		os.RemoveAll(homeDir)
	})

	when("subcommand is invalid", func() {
		it("prints usage", func() {
			cmd := exec.Command(pack, "some-bad-command")
			cmd.Env = append(os.Environ(), "HOME="+homeDir)
			output, _ := cmd.CombinedOutput()
			if !strings.Contains(string(output), `unknown command "some-bad-command" for "pack"`) {
				t.Fatal("Failed to print usage", string(output))
			}
			if !strings.Contains(string(output), `Run 'pack --help' for usage.`) {
				t.Fatal("Failed to print usage", string(output))
			}
		})
	})

	when("build on daemon", func() {
		var (
			sourceCodePath, repoName, containerName, registryContainerName, registryPort string
		)

		it.Before(func() {
			sourceCodePath = filepath.Join("fixtures", "node_app")
			repoName = "http://localhost:5000/some-org/" + randString(10)
			containerName = "test-" + randString(10)
			registryContainerName = "test-registry-" + randString(10)

			cmd := exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", registryContainerName, "registry:2")
			run(t, cmd)

			registryPort = fetchHostPort(t, registryContainerName)
		})
		it.After(func() {
			exec.Command("docker", "kill", containerName).Run()
			exec.Command("docker", "rmi", repoName).Run()
			exec.Command("docker", "kill", registryContainerName).Run()
			exec.Command("docker", "rmi", registryContainerName).Run()
		})

		when("'--publish' flag is not specified'", func() {
			it("builds and exports an image", func() {
				repo := "some-org/" + randString(10)
				repoName := "localhost:" + registryPort + "/" + repo

				cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath)
				cmd.Env = append(os.Environ(), "HOME="+homeDir)
				run(t, cmd)

				run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(2 * time.Second)
				assertEq(t, fetch(t, "http://localhost:"+launchPort), "Buildpacks Worked!")

				t.Log("uses the cache on subsequent run")
				cmd = exec.Command(pack, "build", repoName, "-p", sourceCodePath)
				cmd.Env = append(os.Environ(), "HOME="+homeDir)
				output := run(t, cmd)

				regex := regexp.MustCompile(`moved \d+ packages`)
				if !regex.MatchString(output) {
					t.Fatalf("Build failed to use cache: %s", output)
				}

				t.Log("Checking that registry is empty")
				contents := fetch(t, fmt.Sprintf("http://localhost:%s/v2/_catalog", registryPort))
				if strings.Contains(string(contents), repo) {
					t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
				}
			})
		}, spec.Parallel(), spec.Report(report.Terminal{}))

		when("'--publish' flag is specified", func() {
			it("builds and exports an image", func() {
				repo := "some-org/" + randString(10)
				repoName := fmt.Sprintf("localhost:%s/%s", registryPort, repo)

				t.Log("push v3/packs:run to local registry")
				for _, name := range []string{"detect", "analyze", "build", "export", "run"} {
					run(t, exec.Command("docker", "tag", "packs/v3:"+name, fmt.Sprintf("host-machine.local:%s/packs/v3:%s", registryPort, name)))
				}
				run(t, exec.Command("docker", "tag", "packs/v3:run", fmt.Sprintf("localhost:%s/packs/v3:run", registryPort)))
				run(t, exec.Command("docker", "push", fmt.Sprintf("localhost:%s/packs/v3:run", registryPort)))

				t.Log("run pack build")
				cmd := exec.Command(pack, "build", fmt.Sprintf("host-machine.local:%s/%s", registryPort, repo), "-p", sourceCodePath, "-s", fmt.Sprintf("host-machine.local:%s/packs/v3", registryPort), "--publish")
				cmd.Env = append(os.Environ(), "HOME="+homeDir)
				run(t, cmd)

				t.Log("Checking that registry has contents")
				contents := fetch(t, fmt.Sprintf("http://localhost:%s/v2/_catalog", registryPort))
				if !strings.Contains(string(contents), repo) {
					t.Fatalf("Expected to see image %s in %s", repo, contents)
				}

				t.Log("run image:", repoName)
				run(t, exec.Command("docker", "pull", repoName))
				run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(2 * time.Second)
				assertEq(t, fetch(t, "http://localhost:"+launchPort), "Buildpacks Worked!")
			})
		}, spec.Parallel(), spec.Report(report.Terminal{}))
	})
}

func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute command: %v, %s, %s", cmd.Args, err, output)
	}

	return string(output)
}

func fetch(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to make request to [%s]: %s", url, err)
	}

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to make request to [%s]: %s", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Request returned bad status code: [%d] : %s", resp.StatusCode, contents)
	}

	return string(contents)
}

func fetchHostPort(t *testing.T, dockerID string) string {
	t.Helper()

	output, err := exec.Command(
		"docker",
		"inspect",
		`--format={{range $p, $conf := .NetworkSettings.Ports}} {{(index $conf 0).HostPort}} {{end}}`,
		dockerID,
	).Output()

	if err != nil {
		t.Fatalf("Failed to fetch registry host port: %s", err)
	}

	return strings.TrimSpace(string(output))
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

func findHostMachineIP() (string, error) {
	txt, err := exec.Command("docker", "run", "ubuntu:18.04", "cat", "/proc/net/route").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(txt), "\n") {
		arr := strings.Split(line, "\t")
		if len(arr) > 2 && arr[1] == "00000000" {
			b, err := hex.DecodeString(arr[2])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0]), nil
		}
	}
	return "", errors.New("Could not determine host machine ip")
}
