package policy

import "slices"

func ProjectActions() []Action {
	return []Action{
		ProjectRead, ProjectManage, RunRead, RunTrigger, RunCancel, RunApprove, ArtifactRead,
		EnvironmentManage, EnvironmentPause, EnvironmentBypass, DeploymentApprove, DeploymentRollback, DeploymentCheck, SecretRead, SecretManage, IngestManage,
	}
}

func IsProjectAction(a Action) bool { return slices.Contains(ProjectActions(), a) }

func ProjectRole(org Role, bindings []Role) Role {
	if Rank(org) == 0 {
		return ""
	}
	if AtLeast(org, RoleAdmin) {
		return org
	}
	var best Role
	for _, b := range bindings {
		if Bindable(b) && Rank(b) > Rank(best) {
			best = b
		}
	}
	if best == "" {
		return org
	}
	return best
}

func Project(method, path string, action Action, orgParam, projectParam string) Rule {
	return Rule{Method: method, Path: path, Scope: ScopeProject, Action: action, OrgParam: orgParam, ProjectParam: projectParam}
}

func (r Rule) validProject() bool {
	return IsProjectAction(r.Action) && r.OrgParam != "" && r.ProjectParam != "" && r.OrgParam != r.ProjectParam && r.Runner == 0
}
