package pulse

import (
	"bufio"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchRepos watches local git repos for instant push events.
func WatchRepos(hub *Hub, repos []string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("watch: fsnotify failed", "err", err)
		return
	}

	debounce := make(map[string]time.Time)

	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		abs, err := filepath.Abs(repo)
		if err != nil {
			continue
		}
		logsDir := filepath.Join(abs, ".git", "logs", "refs", "heads")
		_ = watcher.Add(logsDir)
		_ = watcher.Add(filepath.Join(abs, ".git", "logs", "HEAD"))
		slog.Info("watch: monitoring repo", "path", abs)
	}

	pusher := currentUser()

	go func() {
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Op&fsnotify.Write == 0 && ev.Op&fsnotify.Create == 0 {
					continue
				}
				if t, ok := debounce[ev.Name]; ok && time.Since(t) < 200*time.Millisecond {
					continue
				}
				debounce[ev.Name] = time.Now()

				repo, ref, oldSHA, newSHA, ok := parseLogChange(ev.Name)
				if !ok {
					continue
				}
				event := buildWatchEvent(repo, ref, oldSHA, newSHA, pusher)
				hub.Ingest(event)

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("watch: error", "err", err)
			}
		}
	}()
}

func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return "local"
	}
	return u.Username
}

func parseLogChange(path string) (repo, ref, oldSHA, newSHA string, ok bool) {
	path = filepath.Clean(path)
	if !strings.Contains(path, ".git"+string(filepath.Separator)+"logs") {
		return "", "", "", "", false
	}

	gitIdx := strings.Index(path, ".git")
	repo = filepath.Dir(path[:gitIdx])
	logsPrefix := filepath.Join(repo, ".git", "logs", "refs", "heads") + string(filepath.Separator)

	if strings.HasPrefix(path, logsPrefix) {
		branch := strings.TrimPrefix(path, logsPrefix)
		ref = "refs/heads/" + branch
	} else if strings.HasSuffix(path, filepath.Join(".git", "logs", "HEAD")) {
		ref = "HEAD"
	} else {
		return "", "", "", "", false
	}

	oldSHA, newSHA, ok = readLastReflogLine(path)
	if !ok {
		return "", "", "", "", false
	}
	return repo, ref, oldSHA, newSHA, true
}

func readLastReflogLine(path string) (oldSHA, newSHA string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", false
	}
	defer f.Close()

	var last string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		last = sc.Text()
	}
	if last == "" {
		return "", "", false
	}

	fields := strings.Fields(last)
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], fields[1], true
}

func buildWatchEvent(repo, ref, oldSHA, newSHA, pusher string) Event {
	if ref == "HEAD" {
		ref = resolveHEADRef(repo)
	}

	e := NewEvent()
	e.TS = time.Now().UTC()
	e.Repo = repo
	e.Ref = ref
	e.Branch = RefBranch(ref)
	e.OldSHA = shortSHA(oldSHA)
	e.NewSHA = shortSHA(newSHA)
	e.Pusher = pusher
	e.Source = SourceWatch
	e.CommitsDetail = gitLogCommits(repo, oldSHA, newSHA)
	e.Commits = len(e.CommitsDetail)

	zeroSHA := strings.HasPrefix(oldSHA, "0000000") || oldSHA == ""
	if zeroSHA {
		e.Type = TypeBranchCreate
	} else {
		e.Type = TypePush
	}
	return e
}

func resolveHEADRef(repo string) string {
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "HEAD").Output()
	if err != nil {
		return "refs/heads/HEAD"
	}
	return strings.TrimSpace(string(out))
}

func gitLogCommits(repo, oldSHA, newSHA string) []CommitInfo {
	if strings.HasPrefix(oldSHA, "0000000") {
		oldSHA = ""
	}
	args := []string{"-C", repo, "log", "--format=%H|%s", "-n", "30"}
	if oldSHA != "" {
		args = append(args, oldSHA+".."+newSHA)
	} else {
		args = append(args, newSHA)
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, CommitInfo{
			SHA:     shortSHA(parts[0]),
			Message: strings.TrimSpace(parts[1]),
		})
	}
	return commits
}
