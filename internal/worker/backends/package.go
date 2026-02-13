package backends

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// packageNameRe validates package names: alphanumeric plus -.+
var packageNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+_-]*$`)

type PackageBackend struct{}

func init() {
	registerBackend("package", &PackageBackend{})
}

func (p *PackageBackend) Actions() []string {
	return []string{"install", "remove", "update", "upgrade", "info", "list", "search"}
}

func (p *PackageBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "install":
		return p.install(ctx, params)
	case "remove":
		return p.removeAction(ctx, params)
	case "update":
		return p.update(ctx)
	case "upgrade":
		return p.upgrade(ctx)
	case "info":
		return p.info(ctx, params)
	case "list":
		return p.list(ctx, params)
	case "search":
		return p.search(ctx, params)
	default:
		return nil, fmt.Errorf("package: unknown action %q", action)
	}
}

// detectPackageManager returns the available package manager command.
func detectPackageManager() string {
	for _, pm := range []string{"apt-get", "dnf", "yum", "apk"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

func validatePackageName(name string) error {
	if !packageNameRe.MatchString(name) {
		return fmt.Errorf("package: invalid package name %q", name)
	}
	return nil
}

func (p *PackageBackend) install(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("package", "install", "name", params)
	if err != nil {
		return nil, err
	}
	if err := validatePackageName(name); err != nil {
		return nil, err
	}

	pm := detectPackageManager()
	pkg := name
	if v := params["version"]; v != "" {
		switch pm {
		case "apt-get":
			pkg = fmt.Sprintf("%s=%s", name, v)
		case "dnf", "yum":
			pkg = fmt.Sprintf("%s-%s", name, v)
		case "apk":
			pkg = fmt.Sprintf("%s=%s", name, v)
		}
	}

	var args []string
	switch pm {
	case "apt-get":
		args = []string{"install", "-y", pkg}
	case "dnf", "yum":
		args = []string{"install", "-y", pkg}
	case "apk":
		args = []string{"add", pkg}
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	out, err := exec.CommandContext(ctx, pm, args...).CombinedOutput()
	return &Result{Output: truncateOutput(string(out))}, err
}

func (p *PackageBackend) removeAction(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("package", "remove", "name", params)
	if err != nil {
		return nil, err
	}
	if err := validatePackageName(name); err != nil {
		return nil, err
	}

	pm := detectPackageManager()
	var args []string
	switch pm {
	case "apt-get":
		args = []string{"remove", "-y", name}
	case "dnf", "yum":
		args = []string{"remove", "-y", name}
	case "apk":
		args = []string{"del", name}
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	out, err := exec.CommandContext(ctx, pm, args...).CombinedOutput()
	return &Result{Output: truncateOutput(string(out))}, err
}

func (p *PackageBackend) update(ctx context.Context) (*Result, error) {
	pm := detectPackageManager()
	var args []string
	switch pm {
	case "apt-get":
		args = []string{"update"}
	case "dnf", "yum":
		args = []string{"check-update"}
	case "apk":
		args = []string{"update"}
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	out, err := exec.CommandContext(ctx, pm, args...).CombinedOutput()
	// dnf check-update returns exit code 100 when updates are available
	if pm == "dnf" || pm == "yum" {
		return &Result{Output: truncateOutput(string(out))}, nil
	}
	return &Result{Output: truncateOutput(string(out))}, err
}

func (p *PackageBackend) upgrade(ctx context.Context) (*Result, error) {
	pm := detectPackageManager()
	var args []string
	switch pm {
	case "apt-get":
		args = []string{"upgrade", "-y"}
	case "dnf", "yum":
		args = []string{"upgrade", "-y"}
	case "apk":
		args = []string{"upgrade"}
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	out, err := exec.CommandContext(ctx, pm, args...).CombinedOutput()
	return &Result{Output: truncateOutput(string(out))}, err
}

func (p *PackageBackend) info(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("package", "info", "name", params)
	if err != nil {
		return nil, err
	}
	if err := validatePackageName(name); err != nil {
		return nil, err
	}

	pm := detectPackageManager()
	var args []string
	switch pm {
	case "apt-get":
		// Use dpkg for installed info, apt-cache for repo info
		out, err := exec.CommandContext(ctx, "dpkg", "-s", name).CombinedOutput()
		if err != nil {
			out, err = exec.CommandContext(ctx, "apt-cache", "show", name).CombinedOutput()
		}
		return &Result{Output: truncateOutput(string(out))}, err
	case "dnf", "yum":
		args = []string{"info", name}
	case "apk":
		args = []string{"info", name}
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	out, err := exec.CommandContext(ctx, pm, args...).CombinedOutput()
	return &Result{Output: truncateOutput(string(out))}, err
}

func (p *PackageBackend) list(ctx context.Context, params map[string]string) (*Result, error) {
	pm := detectPackageManager()
	pattern := params["pattern"]

	var out []byte
	var err error
	switch pm {
	case "apt-get":
		args := []string{"--installed"}
		if pattern != "" {
			args = append(args, pattern)
		}
		out, err = exec.CommandContext(ctx, "dpkg-query", append([]string{"-W", "-f", "${Package}\t${Version}\n"}, args[:0]...)...).CombinedOutput()
		if pattern != "" {
			// Filter with dpkg-query -W pattern
			out, err = exec.CommandContext(ctx, "dpkg-query", "-W", "-f", "${Package}\t${Version}\n", pattern).CombinedOutput()
		} else {
			out, err = exec.CommandContext(ctx, "dpkg-query", "-W", "-f", "${Package}\t${Version}\n").CombinedOutput()
		}
	case "dnf", "yum":
		args := []string{"list", "installed"}
		if pattern != "" {
			args = append(args, pattern+"*")
		}
		out, err = exec.CommandContext(ctx, pm, args...).CombinedOutput()
	case "apk":
		args := []string{"list", "--installed"}
		if pattern != "" {
			args = append(args, pattern+"*")
		}
		out, err = exec.CommandContext(ctx, "apk", args...).CombinedOutput()
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, err
}

func (p *PackageBackend) search(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("package", "search", "name", params)
	if err != nil {
		return nil, err
	}
	if err := validatePackageName(name); err != nil {
		return nil, err
	}

	pm := detectPackageManager()
	var out []byte
	switch pm {
	case "apt-get":
		out, err = exec.CommandContext(ctx, "apt-cache", "search", name).CombinedOutput()
	case "dnf", "yum":
		out, err = exec.CommandContext(ctx, pm, "search", name).CombinedOutput()
	case "apk":
		out, err = exec.CommandContext(ctx, "apk", "search", name).CombinedOutput()
	default:
		return nil, fmt.Errorf("package: no supported package manager found")
	}

	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, err
}
