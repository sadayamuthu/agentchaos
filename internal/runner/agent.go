package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
)

// presets maps a preset name to its argv template and environment.
var presets = map[string]struct {
	argv []string
	env  map[string]string
}{
	"claude-code": {
		argv: []string{"claude", "-p", "{prompt}", "--mcp-config", "{mcp_config}", "--strict-mcp-config", "--allowedTools", "{allowed_tools}"},
		env:  map[string]string{"MCP_TOOL_TIMEOUT": "30000"},
	},
}

// BuildArgv returns the agent's argv and extra environment. {prompt} and
// {mcp_config} are substituted inside every element; an element that is
// exactly {allowed_tools} expands to one element per server, mcp__<server>.
// The environment is the preset's (if any) overlaid with a.Env.
func BuildArgv(a experiment.Agent, prompt, mcpConfig string, servers []string) ([]string, map[string]string) {
	tmpl := a.Command
	env := map[string]string{}
	if p, ok := presets[a.Preset]; ok {
		tmpl = p.argv
		for k, v := range p.env {
			env[k] = v
		}
	}
	for k, v := range a.Env {
		env[k] = v
	}
	// A single pass keeps text from the prompt from being substituted again.
	rep := strings.NewReplacer("{prompt}", prompt, "{mcp_config}", mcpConfig)
	argv := make([]string, 0, len(tmpl)+len(servers))
	for _, el := range tmpl {
		if el == "{allowed_tools}" {
			for _, s := range servers {
				argv = append(argv, "mcp__"+s)
			}
			continue
		}
		argv = append(argv, rep.Replace(el))
	}
	return argv, env
}

// agentRun is the outcome of running the agent process.
type agentRun struct {
	startErr error
	exitCode int
	timedOut bool
}

// runAgent runs argv in its own process group with cwd dir, environment env
// and output to stdout and stderr, killing the whole group after timeout or
// when ctx is done. Any of the group still alive when the agent exits is
// killed too, and the agent is reaped before runAgent returns.
func runAgent(ctx context.Context, argv []string, dir string, env []string, stdout, stderr *os.File, timeout time.Duration) agentRun {
	if len(argv) == 0 {
		return agentRun{startErr: errors.New("agent: empty command"), exitCode: -1}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil
	// Files rather than writers: the children get the descriptors directly,
	// so Wait does not wait on descendants holding a pipe open.
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	if err := cmd.Start(); err != nil {
		return agentRun{startErr: err, exitCode: -1}
	}
	pgid := cmd.Process.Pid
	_ = cmd.Wait()
	// Kill stragglers (MCP proxies and servers the agent left behind).
	_ = killGroup(pgid)
	return agentRun{
		exitCode: cmd.ProcessState.ExitCode(),
		timedOut: errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil,
	}
}

func killGroup(pgid int) error {
	err := syscall.Kill(-pgid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
