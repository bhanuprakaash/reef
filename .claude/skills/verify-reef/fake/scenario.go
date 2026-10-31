package main

import (
	"regexp"
	"strings"
)

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
	{"name-fail", "", pat(`(?s)^Name this chat Space.*naming-fail`)},
	{"name-login", "", pat(`^Name this chat Space`)},
	{"sam-codeword", "sam", pat(`Remember the codeword plum-31`)},
	{"sam-recall-codeword", "sam", pat(`Which codeword did I just give you`)},
	{"sam-no-codeword", "sam", pat(`Did I give you a codeword`)},
	{"sam-remember-tabs", "sam", pat(`Remember that Ada prefers tabs`)},
	{"sam-recall-tabs", "sam", pat(`What does Ada prefer in Go files`)},
	{"sam-login-plan", "sam", pat(`Ada wants a login page with an email field`)},
	{"sam-summary", "sam", pat(`The user is adding @kit to this conversation`)},
	{"kit-takeover", "kit", pat(`^sam wrote: @kit `)},
	{"flow-lead-start", "lead", pat(`^Work item \d+: Checkout form`)},
	{"flow-developer", "developer", pat(`^lead wrote: @developer build the form`)},
	{"flow-tester", "tester", pat(`^developer wrote: @tester check the form`)},
	{"flow-lead-submit", "lead", pat(`^tester wrote: @lead the form passes`)},
	{"flow-lead-resubmit", "lead", pat(`^Sent back from review by the user:\nADD A PHONE FIELD`)},
	{"flow-propose-kit", "kit", pat(`PROPOSE THE SLOW SEARCH`)},
	{"queue-lead", "lead", pat(`^Work item \d+: `)},
	{"submit-lead", "lead", pat(`(?m)^SUBMIT FOR REVIEW$`)},
	{"propose-deploy", "sam", pat(`^PROPOSE THE FAILING DEPLOY$`)},
	{"skill-draft", "", pat(`^DRAFT A SKILL$`)},
	{"sent-back-lead", "lead", pat(`^Sent back from review by the user:`)},
	{"coordinate-send", "lead", pat(`COORDINATE the login review`)},
	{"coordinate-fail", "lead", pat(`COORDINATE the failing review`)},
	{"read-space", "sam", pat(`Why did we choose sessions in #\[Login\]\(space:1\)`)},
	{"consult-ask", "sam", pat(`CONSULT kit about the login plan`)},
	{"consult-answer", "kit", pat(`sam wrote privately: Please review the login plan`)},
	{"consult-relay", "sam", pat(`^kit replied: `)},
	{"direct-review", "architect", pat(`wrote privately: Please review the login plan`)},
	{"direct-fail", "architect", pat(`wrote privately: Please review the failing plan`)},
	{"direct-noted", "", pat(`(?s)architect wrote privately: .*REVIEWED`)},
	{"hop6-lead", "lead", pat(`RETRY the hop six check`)},
	{"hop6-confirm", "", pat(`wrote privately: Confirm the hop six`)},
	{"hop6-noted", "lead", pat(`^(architect|developer) wrote privately: Confirmed\.`)},
	{"hop6-fail", "lead", pat(`^tester wrote privately: Confirmed\.`)},
	{"pair-lead", "lead", pat(`QUEUE two reviews for the architect`)},
	{"pair-developer", "developer", pat(`^lead wrote: @developer please ask the architect to review the second plan`)},
	{"pair-architect", "architect", pat(`wrote privately: Please review the second plan`)},
	{"order-lead", "lead", pat(`ORDER the build check`)},
	{"order-developer", "developer", pat(`wrote privately: Please confirm the build is green`)},
	{"order-tester", "tester", pat(`(?s)lead wrote: .*@tester please run the order check`)},
	{"order-noted", "lead", pat(`developer wrote privately: Build is green`)},
	{"loop-start", "lead", pat(`COORDINATE the endless review`)},
	{"loop-architect", "architect", pat(`lead wrote: @architect take a look|lead wrote privately: pong`)},
	{"loop-lead", "lead", pat(`architect wrote privately: ping`)},
	{"wrap-propose", "", pat(`Wrap up this Space\.`)},
	{"memory-review", "", pat(`Review your own memory\.`)},
	{"lockout-note", "", pat(`note: we decided to lock an account after 5 failed logins`)},
	{"propose-brief", "", pat(`Call the propose_brief tool once`)},
	{"design-codeword", "lead", pat(`codeword kiwi-77`)},
	{"recall-codeword-decision", "architect", pat(`What codeword did I give you`)},
	{"post-to-space-thread", "architect", pat(`post_to_space tool once with body: thread post`)},
	{"post-to-space-ask", "lead", pat(`post_to_space tool once to ask the architect`)},
	{"thread-reply", "architect", pat(`Which field would you validate first`)},
	{"design-mention", "lead", pat(`(?i)\bDESIGN\b`)},
	{"relay-go", "lead", pat(`^go$`)},
	{"hop-ping", "lead", pat(`^(start|again|architect wrote: @lead pong)$`)},
	{"hop-pong", "architect", pat(`^lead wrote: @architect ping$`)},
	{"work-lead", "lead", pat(`^workpong$`)},
	{"work-developer", "developer", pat(`^(lead wrote: @developer work|tester wrote: @developer again)$`)},
	{"work-tester", "tester", pat(`^developer wrote: @tester check$`)},
	{"talk-lead", "lead", pat(`^(talkpong|developer wrote: @lead pong)$`)},
	{"talk-developer", "developer", pat(`^lead wrote: @developer ping$`)},
	{"relay-tester-woken", "tester", pat(`lead wrote: @tester check this`)},
	{"architect-woken", "architect", pat(`lead wrote: @architect `)},
	{"google-architect-woken", "architect", pat(`@architect sketch the route/handler shape`)},
	{"google-developer-woken", "developer", pat(`@developer please implement per above`)},
	{"google-tester-woken", "tester", pat(`Implemented per the architect's plan`)},
	{"google-lead-final", "lead", pat(`(?s)tester wrote: .*TestGoogleAuthRedirect`)},
	{"run-checks", "tester", pat(`run the checks`)},
	{"webfetch-allowed", "", pat(`WebFetch tool again`)},
	{"webfetch-denied", "", pat(`WebFetch tool to fetch`)},
	{"write-refused", "", pat(`file t\.txt containing the word nope`)},
	{"write-d", "", pat(`file d\.txt containing the word yes`)},
	{"write-file", "", pat(`create new\.txt in the current directory containing the word hi`)},
	{"delete-readme", "", pat(`Bash tool to delete README\.md`)},
	{"f-file", "", pat(`file f\.txt containing the word one`)},
	{"append-commit", "", pat(`Append a line with the word two to a\.txt`)},
	{"branch-switch", "", pat(`git checkout -q -b side`)},
	{"settings-edit-refused", "", pat(`replace the word one with nope`)},
	{"handoff-mention", "", pat(`an at sign immediately followed by the word`)},
	{"google-ask", "", pat(`Add Google login to this app`)},
	{"recall-secret-word", "", pat(`What was the secret word`)},
	{"recall-failed-logins", "lead", pat(`What did we decide about failed logins`)},
	{"story-pineapple", "", pat(`secret word pineapple`)},
	{"story", "", pat(`1500-word story about lighthouses`)},
	{"numbers-600", "", pat(`numbers from 1 to 600`)},
	{"numbers-300", "", pat(`numbers from 1 to 300`)},
	{"one-word", "", pat(`(?i)reply with (exactly )?one(\s+other)? word`)},
	{"subagent-read", "", pat(`Use the Agent tool to start a subagent that reads the file sub\.txt`)},
	{"list-read-summarize", "", pat(`List the files in this directory with a tool`)},
	{"brief-goal", "", pat(`(?i)what is the goal of this work item`)},
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

// _roleRe reads the role out of a Space's first prompt: "You are <role> on work item"
// or, for a Space with no work item, "You are <role>." (space/context.go).
var _roleRe = regexp.MustCompile(`You are (\S+?)(?: on work item|\.\n)`)

// _headerRe is a wake block's header line (space/wake.go), "[Space N, message M]" or
// "[Space N, message M, thread T]", or a direct message's (server/wake.go).
var _headerRe = regexp.MustCompile(`(?m)^\[Space [^\n\]]*\]$`)

// _wroteRe starts a bot's message in a wake: "<role> wrote: " or "<role> wrote privately: ".
var _wroteRe = regexp.MustCompile(`(?m)^\S+ wrote(?: privately)?: `)

// _leadLinesRe is the lines a block may have before its messages.
var _leadLinesRe = regexp.MustCompile(`\A(?:(?:Asked together: |\d+ older messages are not shown)[^\n]*\n)*`)

// textPart is the part of a prompt scenarios match on: the newest message of its last wake
// block, or the whole prompt when it has none (a Space's first prompt, with no wake yet).
// That message ends the block, after reef's notes ("reef wrote: ...") and the older unread
// messages ("New in the Space:"), each list ending in a blank line: it is the last
// bot's "<role> wrote: ", else the user's text after the last blank line.
func textPart(prompt string) string {
	locs := _headerRe.FindAllStringIndex(prompt, -1)
	if len(locs) == 0 {
		return prompt
	}
	rest := strings.TrimPrefix(prompt[locs[len(locs)-1][1]:], "\n")
	wrote := _wroteRe.FindAllStringIndex(rest, -1)
	for i := len(wrote) - 1; i >= 0; i-- {
		if !strings.HasPrefix(rest[wrote[i][0]:], "reef wrote: ") {
			return rest[wrote[i][0]:]
		}
	}
	if i := strings.LastIndex(rest, "\n\n"); i >= 0 {
		rest = rest[i+2:]
	}
	return _leadLinesRe.ReplaceAllString(rest, "")
}
