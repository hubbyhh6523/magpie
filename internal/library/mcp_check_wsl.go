package library

// A command that an agent in WSL starts runs in its distro: `node
// /home/me/tools/x.js` is a Linux program there, which on Windows is
// another node or none at all (loong on Discord: a server his agents in
// WSL start fine was checked on Windows, where it "exited (1)"). So the
// check starts a command where the agents given it start it: on this
// machine for its own agents, and in each distro whose agents have it.
// WSL is gone, so everything is checked here now.

// checkSides are where s's command is checked: "" for this machine. WSL is
// gone, so a distro's agents get no check of their own.
func checkSides(s *Server) []string {
	return []string{""}
}
