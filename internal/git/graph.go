package git

import (
	"fmt"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// graphCommitCap bounds the object scan so a very large repo cannot stall the
// graph endpoint. When hit, Graph.Truncated is set.
const graphCommitCap = 2000

// GraphCommit is one node in the commit graph.
type GraphCommit struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"short_hash"`
	Summary   string    `json:"summary"`
	Author    string    `json:"author"`
	Date      time.Time `json:"date"`
	Parents   []string  `json:"parents"`
	Reachable bool      `json:"reachable"` // reachable from some ref (else abandoned)
	Pushed    bool      `json:"pushed"`    // reachable from a remote-tracking ref
}

// GraphRef is a branch / remote branch / tag label.
type GraphRef struct {
	Name string `json:"name"`
	Type string `json:"type"` // branch | remote | tag
	Hash string `json:"hash"`
}

// Graph is the whole-repository commit graph.
type Graph struct {
	Commits   []GraphCommit `json:"commits"`
	Refs      []GraphRef    `json:"refs"`
	HeadHash  string        `json:"head_hash"`
	Truncated bool          `json:"truncated"`
}

// GetGraph walks every commit object in the repository (so unreachable commits
// appear too) plus all refs, classifying each commit's reachability.
func GetGraph(repo *gogit.Repository) (*Graph, error) {
	g := &Graph{Commits: []GraphCommit{}, Refs: []GraphRef{}}

	if head, err := repo.Head(); err == nil {
		g.HeadHash = head.Hash().String()
	}

	// Collect refs and the tips used for reachability.
	var allTips, remoteTips []plumbing.Hash
	refIter, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}
	_ = refIter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil // skip symbolic refs (HEAD)
		}
		name := ref.Name()
		var typ string
		switch {
		case name.IsBranch():
			typ = "branch"
		case name.IsRemote():
			typ = "remote"
		case name.IsTag():
			typ = "tag"
		default:
			return nil
		}
		g.Refs = append(g.Refs, GraphRef{Name: name.Short(), Type: typ, Hash: ref.Hash().String()})
		allTips = append(allTips, ref.Hash())
		if typ == "remote" {
			remoteTips = append(remoteTips, ref.Hash())
		}
		return nil
	})

	reachable := reachableFrom(repo, allTips)
	pushed := reachableFrom(repo, remoteTips)

	cIter, err := repo.CommitObjects()
	if err != nil {
		return nil, fmt.Errorf("commit objects: %w", err)
	}
	defer cIter.Close()
	_ = cIter.ForEach(func(c *object.Commit) error {
		if len(g.Commits) >= graphCommitCap {
			g.Truncated = true
			return errStopIteration
		}
		parents := make([]string, 0, len(c.ParentHashes))
		for _, p := range c.ParentHashes {
			parents = append(parents, p.String())
		}
		h := c.Hash.String()
		_, isReachable := reachable[c.Hash]
		_, isPushed := pushed[c.Hash]
		g.Commits = append(g.Commits, GraphCommit{
			Hash:      h,
			ShortHash: h[:7],
			Summary:   summaryLine(c.Message),
			Author:    c.Author.Name,
			Date:      c.Author.When,
			Parents:   parents,
			Reachable: isReachable,
			Pushed:    isPushed,
		})
		return nil
	})

	// Topological order — every commit before its parents — with newest-first
	// preference among commits free to emit. The frontend lane layout needs
	// children to precede parents; a plain date sort breaks when commits share
	// a (second-precision) timestamp.
	g.Commits = topoSortCommits(g.Commits)
	return g, nil
}

// topoSortCommits orders commits so each appears before its parents, picking
// the newest-dated commit among those currently free to emit.
func topoSortCommits(commits []GraphCommit) []GraphCommit {
	idx := make(map[string]int, len(commits))
	for i := range commits {
		idx[commits[i].Hash] = i
	}
	childCount := make([]int, len(commits))
	for i := range commits {
		for _, p := range commits[i].Parents {
			if j, ok := idx[p]; ok {
				childCount[j]++
			}
		}
	}
	ready := []int{}
	for i := range commits {
		if childCount[i] == 0 {
			ready = append(ready, i)
		}
	}
	out := make([]GraphCommit, 0, len(commits))
	emitted := make([]bool, len(commits))
	for len(ready) > 0 {
		pick := 0
		for k := 1; k < len(ready); k++ {
			if commits[ready[k]].Date.After(commits[ready[pick]].Date) {
				pick = k
			}
		}
		i := ready[pick]
		ready = append(ready[:pick], ready[pick+1:]...)
		emitted[i] = true
		out = append(out, commits[i])
		for _, p := range commits[i].Parents {
			if j, ok := idx[p]; ok {
				childCount[j]--
				if childCount[j] == 0 {
					ready = append(ready, j)
				}
			}
		}
	}
	for i := range commits { // safety — a DAG should leave none
		if !emitted[i] {
			out = append(out, commits[i])
		}
	}
	return out
}

// reachableFrom returns every commit hash reachable from the given tips.
func reachableFrom(repo *gogit.Repository, tips []plumbing.Hash) map[plumbing.Hash]struct{} {
	set := map[plumbing.Hash]struct{}{}
	stack := append([]plumbing.Hash{}, tips...)
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, seen := set[h]; seen {
			continue
		}
		set[h] = struct{}{}
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		stack = append(stack, c.ParentHashes...)
	}
	return set
}

func summaryLine(msg string) string {
	return strings.SplitN(strings.TrimSpace(msg), "\n", 2)[0]
}
