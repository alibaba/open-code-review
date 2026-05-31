package platform

import "github.com/open-code-review/open-code-review/internal/model"

const (
	PlatformGitLab = "gitlab"
)

const (
	InlineMarker         = "<!-- open-code-review:inline -->"
	SummaryMarker        = "<!-- open-code-review:summary -->"
	RequestChangesMarker = "<!-- open-code-review:request-changes -->"
	PRSummaryStartMarker = "<!-- open-code-review:pr-summary:start -->"
	PRSummaryEndMarker   = "<!-- open-code-review:pr-summary:end -->"
)

type PublishOptions struct {
	Platform         string
	ProjectID        string
	MergeRequestIID  int
	BaseURL          string
	Token            string
	Publish          bool
	PRSummary        bool
	ClearExisting    bool
	ClearInline      bool
	ClearSummary     bool
	NoInline         bool
	NoSummaryComment bool
}

type PublishWarning struct {
	Type    string `json:"type"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type PublishResult struct {
	InlineCreated      int
	InlineFailed       int
	InlineSkipped      int
	SummaryCreated     bool
	SummaryUpdated     bool
	InlineDeleted      int
	SummaryDeleted     int
	DescriptionUpdated bool
	Warnings           []PublishWarning
}

type Publisher interface {
	Publish(comments []model.LlmComment) (*PublishResult, error)
	ClearInline() (*PublishResult, error)
	ClearSummary() (*PublishResult, error)
}
