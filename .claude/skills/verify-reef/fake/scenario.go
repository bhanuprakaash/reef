package main

import "regexp"

// scenario matches a prompt's text part to a recording name. Role "" matches any role.
// Entries are matched in order, so a specific pattern must come before a generic one.
type scenario struct {
	name    string
	role    string
	pattern *regexp.Regexp
}

func pat(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// _scenarios covers the prompts verify-reef's window scenarios (scripts/ui/drive.mjs) and
// features/full-team.md send.
var _scenarios = []scenario{
	{"promote-propose-merge", "", pat(`now part of feature .* already has a brief`)},
	{"promote-propose", "", pat(`This quick session is now feature`)},
	{"wrap-propose", "", pat(`Wrap up this channel\. Call the propose_wrap tool`)},
	{"design-codeword", "lead", pat(`codeword kiwi-77`)},
	{"recall-codeword-decision", "architect", pat(`What codeword did I give you`)},
	{"post-to-channel-thread", "architect", pat(`post_to_channel tool once with body: thread post`)},
	{"post-to-channel-ask", "lead", pat(`post_to_channel tool once to ask the architect`)},
	{"thread-reply", "architect", pat(`Which field would you validate first`)},
	{"design-mention", "lead", pat(`(?i)\bDESIGN\b`)},
	{"relay-go", "lead", pat(`^go$`)},
	{"relay-tester-woken", "tester", pat(`lead wrote: @tester check this`)},
	{"architect-woken", "architect", pat(`lead wrote: @architect `)},
	{"google-architect-woken", "architect", pat(`Handed the brief to @architect to sketch the`)},
	{"google-developer-woken", "developer", pat(`@developer please implement per above`)},
	{"google-tester-woken", "tester", pat(`Implemented per the architect's plan`)},
	{"google-lead-final", "lead", pat(`(?s)tester wrote: .*TestGoogleAuthRedirect`)},
	{"run-checks", "tester", pat(`run the checks`)},
	{"write-refused", "", pat(`file t\.txt containing the word nope`)},
	{"write-d", "", pat(`file d\.txt containing the word yes`)},
	{"write-file", "", pat(`create new\.txt in the current directory containing the word hi`)},
	{"f-file", "", pat(`file f\.txt containing the word one`)},
	{"append-commit", "", pat(`Append a line with the word two to a\.txt`)},
	{"branch-switch", "", pat(`git checkout -q -b side`)},
	{"settings-edit-refused", "", pat(`replace the word one with nope`)},
	{"gate-mention", "", pat(`an at sign immediately followed by the word`)},
	{"google-ask", "", pat(`Add Google login to this app`)},
	{"recall-secret-word", "", pat(`What was the secret word`)},
	{"story-pineapple", "", pat(`secret word pineapple`)},
	{"story", "", pat(`1500-word story about lighthouses`)},
	{"numbers-600", "", pat(`numbers from 1 to 600`)},
	{"numbers-300", "", pat(`numbers from 1 to 300`)},
	{"one-word", "", pat(`(?i)reply with (exactly )?one(\s+other)? word`)},
	{"list-read-summarize", "", pat(`List the files in this directory with a tool`)},
	{"brief-goal", "", pat(`(?i)what is the goal of this feature`)},
	{"say-ok-one-word", "", pat(`Say ok in one word`)},
	{"say-fine-one-word", "", pat(`Say fine in one word`)},
	{"rollback-hello", "", pat(`^Say hello\. Answer in one or two sentences, use no tools\.\s*$`)},
	{"rollback-files-changed", "", pat(`Did anything change in the files since your last answer`)},
	{"short-answer", "", pat(`Answer in one or two sentences, use no tools\.\s*$`)},
}

// matchScenario returns the name of the first scenario whose role fits role and whose
// pattern matches text, or "" when none does.
func matchScenario(role, text string) string {
	for _, s := range _scenarios {
		if s.role != "" && s.role != role {
			continue
		}
		if s.pattern.MatchString(text) {
			return s.name
		}
	}
	return ""
}

// _roleRe reads the role out of a channel's first prompt: "You are the <role> on feature"
// or "You are the <role> in project" (channel/context.go).
var _roleRe = regexp.MustCompile(`You are the (\S+) (?:on feature|in project)`)

// _headerRe is a wake's own header line (server/wake.go), "[channel N, message M]" or
// "[channel N, message M, thread T]".
var _headerRe = regexp.MustCompile(`(?m)^\[channel [^\n\]]*\]$`)

// textPart is the part of a prompt scenarios match on: the text after its last header
// line, or the whole prompt when it has none (a channel's first prompt, with no wake yet).
func textPart(prompt string) string {
	locs := _headerRe.FindAllStringIndex(prompt, -1)
	if len(locs) == 0 {
		return prompt
	}
	last := locs[len(locs)-1]
	rest := prompt[last[1]:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	return rest
}
