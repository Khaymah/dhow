package keys

import (
	"github.com/google/uuid"
)

type OwnerKind string

const (
	OwnerInstance     OwnerKind = "instance"
	OwnerOrganization OwnerKind = "organization"
	OwnerProject      OwnerKind = "project"
	OwnerEnvironment  OwnerKind = "environment"
	OwnerUser         OwnerKind = "user"
)

type Owner struct {
	Kind OwnerKind
	ID   uuid.UUID
}

func InstanceOwner() Owner { return Owner{Kind: OwnerInstance} }

func OrganizationOwner(id uuid.UUID) Owner { return Owner{Kind: OwnerOrganization, ID: id} }

func ProjectOwner(id uuid.UUID) Owner { return Owner{Kind: OwnerProject, ID: id} }

func EnvironmentOwner(id uuid.UUID) Owner { return Owner{Kind: OwnerEnvironment, ID: id} }

func UserOwner(id uuid.UUID) Owner { return Owner{Kind: OwnerUser, ID: id} }

func (o Owner) Valid() bool {
	switch o.Kind {
	case OwnerInstance:
		return o.ID == uuid.Nil
	case OwnerOrganization, OwnerProject, OwnerEnvironment, OwnerUser:
		return o.ID != uuid.Nil
	default:
		return false
	}
}

func (o Owner) String() string {
	if o.Kind == OwnerInstance {
		return string(o.Kind)
	}
	return string(o.Kind) + ":" + o.ID.String()
}

func (o Owner) idPtr() *uuid.UUID {
	if o.Kind == OwnerInstance {
		return nil
	}
	id := o.ID
	return &id
}

func ownerFrom(kind string, id *uuid.UUID) Owner {
	o := Owner{Kind: OwnerKind(kind)}
	if id != nil {
		o.ID = *id
	}
	return o
}
