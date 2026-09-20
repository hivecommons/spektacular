package migrate

import "gopkg.in/yaml.v3"

// repo1to2 gives an unversioned repo.yaml a format version. The format is
// otherwise unchanged; the engine stamps `schema` and `written_by`.
var repo1to2 = Step{
	Kind:        KindRepo,
	From:        1,
	Description: "record format version",
	Run: func(*StepContext, *yaml.Node) ([]Action, error) {
		return nil, nil
	},
}
