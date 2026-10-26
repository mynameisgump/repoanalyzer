package stats

import (
	"sort"
	"strings"

	"github.com/mynameisgump/repoanalyzer/internal/gitlog"
)

// Identities merges the names and emails commits were authored under into
// people: two commits are by the same person when they share a name or an
// email, ignoring case, and the merge is transitive ("Dax Raad" <d@x.io>,
// "Dax" <d@x.io> and "Dax" <m@y.io> are one person). git log has already
// applied .mailmap, which wins where it disagrees.
//
// Each person is shown under the name they committed with most often (the
// most recent one on a tie) and the email they used most.
type Identities struct {
	byName  map[string]*Person // lowercased name -> person
	byEmail map[string]*Person // lowercased email -> person
}

// Person is one contributor's display name, email and other names.
type Person struct {
	Name  string
	Email string
	// Aliases are the other names the person committed under, most used first,
	// not counting spellings that differ only in case.
	Aliases []string
}

// spelling is how often one name or email was used, with its first spelling.
type spelling struct {
	text  string
	count int
	seen  int // index of the first commit using it; git log is newest first
}

// more orders spellings most used first, then most recently seen.
func (s *spelling) more(o *spelling) bool {
	if s.count != o.count {
		return s.count > o.count
	}
	return s.seen < o.seen
}

// Identify works out who authored commits; see Identities.
func Identify(commits []gitlog.Commit) *Identities {
	// Union-find over "n:" name and "e:" email keys, joined by each commit.
	parent := map[string]string{}
	var find func(string) string
	find = func(k string) string {
		p, ok := parent[k]
		if !ok {
			parent[k] = k
			return k
		}
		if p != k {
			p = find(p)
			parent[k] = p
		}
		return p
	}
	union := func(a, b string) {
		if ra, rb := find(a), find(b); ra != rb {
			parent[ra] = rb
		}
	}
	tally := func(m map[string]*spelling, key, text string, seen int) {
		if m[key] == nil {
			m[key] = &spelling{text: text, seen: seen}
		}
		m[key].count++
	}

	names, emails := map[string]*spelling{}, map[string]*spelling{}
	for i, c := range commits {
		n := "n:" + strings.ToLower(c.AuthorName)
		tally(names, n, c.AuthorName, i)
		find(n)
		if c.AuthorEmail != "" {
			e := "e:" + strings.ToLower(c.AuthorEmail)
			tally(emails, e, c.AuthorEmail, i)
			union(n, e)
		}
	}

	// Every name in a set is one person, shown under the most-used name.
	byRoot := map[string][]*spelling{}
	for key, s := range names {
		root := find(key)
		byRoot[root] = append(byRoot[root], s)
	}
	people := map[string]*Person{}
	for root, ss := range byRoot {
		sort.Slice(ss, func(i, j int) bool { return ss[i].more(ss[j]) })
		p := &Person{Name: ss[0].text}
		for _, s := range ss[1:] {
			p.Aliases = append(p.Aliases, s.text)
		}
		people[root] = p
	}
	ids := &Identities{byName: map[string]*Person{}, byEmail: map[string]*Person{}}
	for key := range names {
		ids.byName[strings.TrimPrefix(key, "n:")] = people[find(key)]
	}
	best := map[string]*spelling{} // root -> most-used email
	for key, s := range emails {
		root := find(key)
		ids.byEmail[strings.TrimPrefix(key, "e:")] = people[root]
		if b := best[root]; b == nil || s.more(b) {
			best[root] = s
		}
	}
	for root, s := range best {
		people[root].Email = s.text
	}
	return ids
}

// Person is who authored c. A nil Identities, or an author Identify never saw,
// gives the commit's own name and email.
func (ids *Identities) Person(c gitlog.Commit) Person {
	if ids != nil {
		if p := ids.byEmail[strings.ToLower(c.AuthorEmail)]; p != nil && c.AuthorEmail != "" {
			return *p
		}
		if p := ids.byName[strings.ToLower(c.AuthorName)]; p != nil {
			return *p
		}
	}
	return Person{Name: c.AuthorName, Email: c.AuthorEmail}
}

// Name is the display name of whoever authored c.
func (ids *Identities) Name(c gitlog.Commit) string {
	return ids.Person(c).Name
}
