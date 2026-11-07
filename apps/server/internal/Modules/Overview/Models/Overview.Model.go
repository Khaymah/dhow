package models

type Jobs struct {
	Running        int
	Queued         int
	QueuedNoRunner int
	FailedLast24h  int
}

type Runners struct {
	Online  int
	Offline int
	Paused  int
}

type Invitations struct {
	Pending int
}

type Overview struct {
	Jobs          *Jobs
	Runners       *Runners
	Invitations   *Invitations
	Approvals     *Approvals
	Deployments   *Deployments
	Notifications *Notifications
}

type Deployments struct {
	Running        int
	FailedLast24h  int
	RollbackFailed int
	Pending        int
}

type Notifications struct {
	Dead int
}

type Approvals struct {
	Pending int
}
