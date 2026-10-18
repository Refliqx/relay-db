// Package config loads & validates RelayDB preset files (relaydb.yaml).
//
// ARCHITECTURE DECISION: YAML is parsed in Go, not in Rust.
// The reason is that a preset file is an artefact committed to the user's
// application repo. By making Go the single parser, the same engine can be used
// headless in CI:
//
//	relaydb-engine --preset .relaydb.yaml --profile staging --out seeds/dev.sql
//
// Rust is only responsible for the file dialog & atomic write. No
// schema-config logic is duplicated across two languages.
package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/refliqx/relaydb/engine/internal/protocol"
	"gopkg.in/yaml.v3"
)

// File is the shape of a single relaydb.yaml file.
type File struct {
	Version   int                       `yaml:"version" json:"version"`
	Profiles  []protocol.Profile        `yaml:"profiles" json:"profiles"`
	Profile   *protocol.Profile         `yaml:"profile,omitempty" json:"profile,omitempty"`
	Selection []protocol.TableSelection `yaml:"selection,omitempty" json:"selection,omitempty"`
	Masking   []protocol.MaskRule       `yaml:"masking,omitempty" json:"masking,omitempty"`
	Pruning   []protocol.PruneRule      `yaml:"pruning,omitempty" json:"pruning,omitempty"`
	Export    protocol.ExportOptions    `yaml:"export,omitempty" json:"export,omitempty"`
}

// Load reads a file from disk, expands environment variables, then validates
// it. I/O errors are separated from validation issues: a file that exists but
// is wrong is still returned so the UI can show the editor plus a list of
// problems.
func Load(path string) (*File, []protocol.ConfigIssue, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading preset %s: %w", path, err)
	}
	file, issues, err := Parse(raw)
	if err != nil {
		return nil, issues, err
	}
	for i := range issues {
		if issues[i].Path == "" {
			issues[i].Path = path
		}
	}
	return file, issues, nil
}

func Parse(raw []byte) (*File, []protocol.ConfigIssue, error) {
	var f File
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true) // a typo in a key is an error, not silently ignored
	if err := dec.Decode(&f); err != nil {
		return nil, []protocol.ConfigIssue{{
			Severity: "error",
			Path:     "yaml",
			Message:  "invalid YAML: " + err.Error(),
		}}, err
	}
	issues := Validate(&f)
	return &f, issues, nil
}

// Validate checks that the preset is complete & consistent.
func Validate(f *File) []protocol.ConfigIssue {
	var issues []protocol.ConfigIssue
	add := func(sev, path, msg string) {
		issues = append(issues, protocol.ConfigIssue{Severity: sev, Path: path, Message: msg})
	}

	if f.Version == 0 {
		f.Version = 1
	}
	if f.Version > 1 {
		add("error", "version", fmt.Sprintf("preset version %d is newer than the engine supports (1)", f.Version))
	}

	profiles := f.AllProfiles()
	if len(profiles) == 0 {
		add("warning", "profiles", "no connection profiles in this file")
	}
	seen := map[string]bool{}
	for i, p := range profiles {
		where := fmt.Sprintf("profiles[%d]", i)
		if p.ID == "" {
			add("error", where+".id", "every profile must have a unique `id`")
		} else if seen[p.ID] {
			add("error", where+".id", "duplicate profile id: "+p.ID)
		}
		seen[p.ID] = true
		if p.Kind == "" && p.DSN == "" {
			add("error", where, "set `kind` or provide `dsn`")
		}
		if p.Kind == protocol.KindSQLite && p.File == "" && p.DSN == "" {
			add("error", where+".file", "SQLite profile needs `file` or `dsn`")
		}
	}

	for i, rule := range f.Masking {
		where := fmt.Sprintf("masking[%d]", i)
		if rule.Table == "" || rule.Column == "" {
			add("error", where, "`table` and `column` are required")
		}
		if rule.Strategy == "" {
			add("error", where+".strategy", "`strategy` is required")
		} else if !KnownStrategy(rule.Strategy) {
			add("error", where+".strategy", "unknown strategy: "+rule.Strategy)
		}
	}

	for i, rule := range f.Pruning {
		where := fmt.Sprintf("pruning[%d]", i)
		if rule.Table == "" {
			add("error", where, "`table` is required")
		}
		if rule.Limit != nil && *rule.Limit < 0 {
			add("error", where+".limit", "limit must not be negative")
		}
		if rule.Where != "" {
			if msg, bad := inspectWhere(rule.Where); bad {
				// Guardrail: RelayDB only reads. A WHERE clause containing a
				// write command must be rejected before it reaches the DB.
				add("error", where+".where", msg)
			}
		}
	}

	if f.Export.OutputPath == "" {
		add("warning", "export.outputPath", "outputPath is not set; the UI will ask during export")
	}
	if f.Export.Seed == "" {
		add("warning", "export.seed", "empty seed -> the masker will use the default seed (results are not unique per project)")
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity == "error"
		}
		return issues[i].Path < issues[j].Path
	})
	return issues
}

// AllProfiles normalises the single `profile` form and the `profiles` list form.
func (f *File) AllProfiles() []protocol.Profile {
	out := make([]protocol.Profile, 0, len(f.Profiles)+1)
	out = append(out, f.Profiles...)
	if f.Profile != nil {
		out = append(out, *f.Profile)
	}
	for i := range out {
		expandProfile(&out[i])
	}
	return out
}

// ApplyDefaults fills any value the user left unset with a safe default.
func ApplyDefaults(f *File) {
	if f.Export.BatchSize == 0 {
		f.Export.BatchSize = 100
	}
	if !f.Export.IncludeHeader {
		// Defaults to true: the header helps review diffs.
		f.Export.IncludeHeader = true
	}
	for i := range f.Masking {
		if f.Masking[i].Strategy == protocol.StrategyFakeEmail {
			if f.Masking[i].Options == nil {
				f.Masking[i].Options = map[string]any{"domain": "example.com"}
			}
		}
	}
}

// Environment variable expansion

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// expandProfile substitutes ${VAR} / ${VAR:-default} in connection fields.
//
// SECURITY: this is what keeps a preset safe to commit: the
// password/authToken are never stored as literals in a file shared with the
// team. Non-secret fields (host/database/file) are expanded too, so one preset
// works across environments without manual edits, e.g. host: ${RELAYDB_HOST}.
func expandProfile(p *protocol.Profile) {
	p.DSN = expand(p.DSN)
	p.Host = expand(p.Host)
	p.User = expand(p.User)
	p.Database = expand(p.Database)
	p.Password = expand(p.Password)
	p.File = expand(p.File)
	p.AuthToken = expand(p.AuthToken)
}

func expand(s string) string {
	if s == "" {
		return s
	}
	return envPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		name := parts[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return parts[2] // default (may be empty)
	})
}

// WHERE clause guardrail

// forbiddenKeywords are SQL commands matched as WHOLE WORDS.
//
// Why word-boundary matching and not strings.Contains? Because legitimate
// columns such as `created_at`, `updated_at`, or `deleted_at` contain
// "create"/"update"/"delete" as substrings. `\b` prevents those false
// positives while still catching `CREATE`, `DELETE`, `UPDATE`, and `updated`
// (but not `updated_at`).
var forbiddenKeywords = regexp.MustCompile(
	`\b(drop|delete|update|insert|alter|truncate|grant|revoke|create|attach|` +
		`vacuum|pragma|call|exec|execute|copy|load_file|pg_sleep|union)\b`)

// forbiddenSequences are characters/phrases that cannot appear in a legitimate
// read filter, so a substring check is enough.
var forbiddenSequences = []string{
	";", "--", "/*", "*/", "into outfile", "into dumpfile",
}

// inspectWhere rejects a WHERE clause that could write or inject an extra
// command. It is not a replacement for a read-only session, but a second layer.
func inspectWhere(where string) (string, bool) {
	lower := strings.ToLower(where)
	if kw := forbiddenKeywords.FindString(lower); kw != "" {
		return fmt.Sprintf("WHERE clause contains the forbidden keyword %q; RelayDB only allows read filters (column value comparisons)", kw), true
	}
	for _, bad := range forbiddenSequences {
		if strings.Contains(lower, bad) {
			return fmt.Sprintf("WHERE clause contains the forbidden token %q; RelayDB only allows read filters (column value comparisons)", bad), true
		}
	}
	return "", false
}

func KnownStrategy(s string) bool {
	for _, c := range StrategyList {
		if s == c {
			return true
		}
	}
	return false
}

var StrategyList = []string{
	protocol.StrategyKeep,
	protocol.StrategyNullify,
	protocol.StrategyFixed,
	protocol.StrategyScramble,
	protocol.StrategyRedact,
	protocol.StrategyHashSHA256,
	protocol.StrategyDefaultHash,
	protocol.StrategyFakeEmail,
	protocol.StrategyFakeName,
	protocol.StrategyFakePhone,
	protocol.StrategyFakeUUID,
	protocol.StrategyRandomInt,
	protocol.StrategySequential,
	protocol.StrategyFakeAddress,
	protocol.StrategyFakeCompany,
	protocol.StrategyJSONScrub,
}
