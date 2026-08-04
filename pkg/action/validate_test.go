package action_test

import (
	"testing"

	"github.com/semx/helmtide/pkg/action"
	"github.com/stretchr/testify/suite"
)

type ValidateTestSuite struct {
	suite.Suite
}

func TestValidateTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ValidateTestSuite))
}

func (ts *ValidateTestSuite) TestCmd() {
	s := &action.Validate{}
	cmd := s.Cmd()

	ts.Require().NotNil(cmd)
	ts.Require().NotEmpty(cmd.Name)
}
