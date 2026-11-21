package library

// magpie's own image server for an agent in WSL (#900) is gone, so an agent
// in WSL can't start a Windows program at all any more: side says so.

import (
	"fmt"
	"strings"
)

// isSelf is whether s is magpie's own image server, whatever its binary.
func isSelf(s *Server) bool {
	return !s.Remote() && len(s.Args) >= 2 && s.Args[0] == "mcp" && s.Args[1] == "image"
}

// side is s as the file's agent is given it: an error says why the agent
// can't start s.
func (f *mcpFile) side(s *Server) (*Server, error) {
	if !f.WSL || s.Remote() || !windowsPath.MatchString(s.Command) {
		return s, nil
	}
	if isSelf(s) {
		return nil, fmt.Errorf("magpie's own image server is gone, so an agent in WSL can't start it")
	}
	return nil, fmt.Errorf("it runs a Windows program (%s), which an agent in WSL can't start: give it a command WSL has", s.Command)
}

// linuxHome is a WSL agent's $HOME as the distro spells it, from where
// magpie opens it: \\wsl.localhost\Ubuntu\home\me is /home/me.
func linuxHome(local string) string {
	for _, pre := range []string{`\\wsl.localhost\`, `\\wsl$\`} {
		if len(local) > len(pre) && strings.EqualFold(local[:len(pre)], pre) {
			rest := local[len(pre):]
			if i := strings.IndexByte(rest, '\\'); i >= 0 {
				return strings.ReplaceAll(rest[i:], `\`, "/")
			}
			return "/"
		}
	}
	return ""
}
