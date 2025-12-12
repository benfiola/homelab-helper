package process

import (
	"context"
	"os/exec"

	"github.com/benfiola/homelab-helper/internal/logging"
)

func Output(ctx context.Context, command []string) (string, error) {
	logger := logging.FromContext(ctx)
	logger.Debug("executing command", "command", command)

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	outputBytes, err := cmd.Output()
	output := string(outputBytes)
	return output, err
}
