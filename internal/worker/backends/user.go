package backends

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// userNameRe validates usernames: alphanumeric plus - and _
var userNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,31}$`)

// reservedUsers are system-critical accounts that cannot be created/deleted.
var reservedUsers = map[string]bool{
	"root": true, "daemon": true, "bin": true, "sys": true,
	"sync": true, "games": true, "man": true, "mail": true,
	"news": true, "uucp": true, "proxy": true, "www-data": true,
	"nobody": true, "systemd-network": true, "systemd-resolve": true,
	"messagebus": true, "sshd": true,
}

type UserBackend struct{}

func init() {
	registerBackend("user", &UserBackend{})
}

func (u *UserBackend) Actions() []string {
	return []string{"list", "exists", "create", "delete", "groups", "group_create"}
}

func (u *UserBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "list":
		return u.list(params)
	case "exists":
		return u.exists(params)
	case "create":
		return u.create(ctx, params)
	case "delete":
		return u.deleteUser(ctx, params)
	case "groups":
		return u.groups(ctx, params)
	case "group_create":
		return u.groupCreate(ctx, params)
	default:
		return nil, fmt.Errorf("user: unknown action %q", action)
	}
}

func validateUserName(name string) error {
	if !userNameRe.MatchString(name) {
		return fmt.Errorf("user: invalid username %q (alphanumeric, -, _ only, max 32 chars)", name)
	}
	if reservedUsers[name] {
		return fmt.Errorf("user: cannot modify reserved user %q", name)
	}
	return nil
}

type userInfo struct {
	Name   string   `json:"name"`
	UID    int      `json:"uid"`
	GID    int      `json:"gid"`
	Home   string   `json:"home"`
	Shell  string   `json:"shell"`
	Groups []string `json:"groups,omitempty"`
}

func (u *UserBackend) list(params map[string]string) (*Result, error) {
	systemOnly := params["system"] == "true"

	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, fmt.Errorf("user list: %w", err)
	}
	defer file.Close()

	var users []userInfo
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 7 {
			continue
		}

		uid, _ := strconv.Atoi(fields[2])
		gid, _ := strconv.Atoi(fields[3])

		if systemOnly && uid >= 1000 {
			continue
		}
		if !systemOnly && uid < 1000 && uid != 0 {
			continue
		}

		users = append(users, userInfo{
			Name:  fields[0],
			UID:   uid,
			GID:   gid,
			Home:  fields[5],
			Shell: fields[6],
		})
	}

	data, err := json.Marshal(users)
	if err != nil {
		return nil, fmt.Errorf("user list: %w", err)
	}
	return &Result{Output: string(data)}, nil
}

func (u *UserBackend) exists(params map[string]string) (*Result, error) {
	name, err := requireParam("user", "exists", "name", params)
	if err != nil {
		return nil, err
	}

	_, lookupErr := exec.Command("id", name).CombinedOutput()
	if lookupErr != nil {
		return &Result{Output: "false"}, nil
	}
	return &Result{Output: "true"}, nil
}

func (u *UserBackend) create(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("user", "create", "name", params)
	if err != nil {
		return nil, err
	}
	if err := validateUserName(name); err != nil {
		return nil, err
	}

	args := []string{}
	if params["system"] == "true" {
		args = append(args, "--system")
	}
	if uid := params["uid"]; uid != "" {
		args = append(args, "--uid", uid)
	}

	shell := "/usr/sbin/nologin"
	if s := params["shell"]; s != "" {
		shell = s
	}
	args = append(args, "--shell", shell, name)

	out, err := exec.CommandContext(ctx, "useradd", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("user create: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return &Result{Output: fmt.Sprintf("created user %s", name)}, nil
}

func (u *UserBackend) deleteUser(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("user", "delete", "name", params)
	if err != nil {
		return nil, err
	}
	if err := validateUserName(name); err != nil {
		return nil, err
	}

	// Refuse to delete low-UID users
	idOut, err := exec.CommandContext(ctx, "id", "-u", name).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("user delete: user %q not found", name)
	}
	uid, _ := strconv.Atoi(strings.TrimSpace(string(idOut)))
	if uid < 1000 {
		return nil, fmt.Errorf("user delete: refusing to delete user %q with UID %d (< 1000)", name, uid)
	}

	out, err := exec.CommandContext(ctx, "userdel", name).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("user delete: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return &Result{Output: fmt.Sprintf("deleted user %s", name)}, nil
}

func (u *UserBackend) groups(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("user", "groups", "name", params)
	if err != nil {
		return nil, err
	}

	out, err := exec.CommandContext(ctx, "groups", name).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("user groups: %w", err)
	}

	// Output: "username : group1 group2 group3"
	parts := strings.SplitN(strings.TrimSpace(string(out)), ":", 2)
	var groupList []string
	if len(parts) == 2 {
		for _, g := range strings.Fields(parts[1]) {
			groupList = append(groupList, g)
		}
	}

	result := map[string]any{
		"user":   name,
		"groups": groupList,
	}
	return jsonResult(result)
}

func (u *UserBackend) groupCreate(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("user", "group_create", "name", params)
	if err != nil {
		return nil, err
	}
	if !userNameRe.MatchString(name) {
		return nil, fmt.Errorf("user group_create: invalid group name %q", name)
	}

	args := []string{}
	if gid := params["gid"]; gid != "" {
		args = append(args, "--gid", gid)
	}
	args = append(args, name)

	out, err := exec.CommandContext(ctx, "groupadd", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("user group_create: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return &Result{Output: fmt.Sprintf("created group %s", name)}, nil
}
