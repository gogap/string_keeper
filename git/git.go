package git

import (
	"os/exec"
)

type Git struct {
	repoPath string
}

func NewGit(repoPath string) *Git {
	return &Git{
		repoPath: repoPath,
	}
}

func (p *Git) Pull() (data []byte, err error) {
	return p.run("-C", p.repoPath, "pull")
}

func (p *Git) CatBlobFile(filePath, revision string) (data []byte, err error) {
	return p.run("-C", p.repoPath, "cat-file", "blob", revision+":"+filePath)
}

func (p *Git) Status() (output []byte, err error) {
	return p.run("-C", p.repoPath, "status", "--short")
}

func (p *Git) run(subcmd string, arg ...string) (output []byte, err error) {
	arg = prependArg(subcmd, arg)
	cmd := exec.Command("git", arg...)
	output, err = cmd.CombinedOutput()
	return
}

func prependArg(pre string, arg []string) []string {
	buffer := make([]string, 1)
	buffer[0] = pre
	return append(buffer, arg...)
}
