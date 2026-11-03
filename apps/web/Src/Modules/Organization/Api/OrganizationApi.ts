import { ApiClient, request, requestVoid } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';
import type { TBindableRole, TOrganization, TRole, TSecondFactorRule } from '@/Core/Auth/AuthTypes';

type TSchemas = components['schemas'];

export type TTeam = TSchemas['Team'];
export type TTeamDetail = TSchemas['TeamDetail'];
export type TTeamChange = TSchemas['TeamChange'];
export type TInvitation = TSchemas['Invitation'];
export type TInvitationIssued = TSchemas['InvitationIssued'];
export type TMemberChange = TSchemas['MemberChange'];

const org = (id: string) => ({ path: { id } });

export const getOrganization = (id: string): Promise<TOrganization> =>
  request(() => ApiClient.GET('/organizations/{id}', { params: org(id) }));

export const renameOrganization = (id: string, name: string): Promise<TOrganization> =>
  request(() => ApiClient.PATCH('/organizations/{id}', { params: org(id), body: { Name: name } }));

export const createOrganization = (name: string, slug: string): Promise<TOrganization> =>
  request(() => ApiClient.POST('/organizations', { body: { Name: name, Slug: slug } }));

export const leaveOrganization = (id: string): Promise<void> =>
  requestVoid(() => ApiClient.POST('/organizations/{id}/leave', { params: org(id) }));

export const transferOwnership = (id: string, userId: string, factor: TCurrentFactor): Promise<TMemberChange> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/transfer-ownership', { params: org(id), body: { UserId: userId, CurrentFactor: factor } }),
  );

export const changeSecondFactorRule = (id: string, rule: TSecondFactorRule, factor: TCurrentFactor): Promise<TOrganization> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/second-factor-rule', { params: org(id), body: { RequireSecondFactor: rule, CurrentFactor: factor } }),
  );

export const changeLogRetention = (id: string, days: number | null, factor: TCurrentFactor): Promise<TOrganization> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/log-retention', { params: org(id), body: { LogRetentionDays: days, CurrentFactor: factor } }),
  );

const member = (id: string, userId: string) => ({ path: { id, userId } });

export const changeRole = (id: string, userId: string, role: TRole, factor?: TCurrentFactor): Promise<TMemberChange> =>
  request(() =>
    ApiClient.PATCH('/organizations/{id}/members/{userId}', {
      params: member(id, userId),
      body: factor === undefined ? { Role: role } : { Role: role, CurrentFactor: factor },
    }),
  );

export const removeMember = (id: string, userId: string): Promise<TMemberChange> =>
  request(() => ApiClient.DELETE('/organizations/{id}/members/{userId}', { params: member(id, userId) }));

export const resetMemberMfa = (id: string, userId: string, factor: TCurrentFactor): Promise<TMemberChange> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/members/{userId}/mfa-reset', { params: member(id, userId), body: { CurrentFactor: factor } }),
  );

export const listTeams = (id: string): Promise<TTeam[]> => request(() => ApiClient.GET('/organizations/{id}/teams', { params: org(id) }));

const team = (id: string, teamId: string) => ({ path: { id, teamId } });

export const getTeam = (id: string, teamId: string): Promise<TTeamDetail> =>
  request(() => ApiClient.GET('/organizations/{id}/teams/{teamId}', { params: team(id, teamId) }));

export const createTeam = (id: string, name: string, description: string, role?: TBindableRole): Promise<TTeamDetail> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/teams', {
      params: org(id),
      body: role === undefined ? { Name: name, Description: description } : { Name: name, Description: description, Role: role },
    }),
  );

export const updateTeam = (id: string, teamId: string, name: string, description: string): Promise<TTeamDetail> =>
  request(() =>
    ApiClient.PATCH('/organizations/{id}/teams/{teamId}', { params: team(id, teamId), body: { Name: name, Description: description } }),
  );

export const deleteTeam = (id: string, teamId: string): Promise<void> =>
  requestVoid(() => ApiClient.DELETE('/organizations/{id}/teams/{teamId}', { params: team(id, teamId) }));

export const addTeamMember = (id: string, teamId: string, userId: string): Promise<TTeamChange> =>
  request(() => ApiClient.POST('/organizations/{id}/teams/{teamId}/members', { params: team(id, teamId), body: { UserId: userId } }));

export const removeTeamMember = (id: string, teamId: string, userId: string): Promise<TTeamChange> =>
  request(() =>
    ApiClient.DELETE('/organizations/{id}/teams/{teamId}/members/{userId}', { params: { path: { id, teamId, userId } } }),
  );

export const bindTeamRole = (id: string, teamId: string, role: TBindableRole): Promise<TTeamChange> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/teams/{teamId}/role-bindings', { params: team(id, teamId), body: { Scope: 'org', Role: role } }),
  );

export const changeTeamRole = (id: string, teamId: string, bindingId: string, role: TBindableRole): Promise<TTeamChange> =>
  request(() =>
    ApiClient.PATCH('/organizations/{id}/teams/{teamId}/role-bindings/{bindingId}', {
      params: { path: { id, teamId, bindingId } },
      body: { Role: role },
    }),
  );

export const unbindTeamRole = (id: string, teamId: string, bindingId: string): Promise<TTeamChange> =>
  request(() =>
    ApiClient.DELETE('/organizations/{id}/teams/{teamId}/role-bindings/{bindingId}', { params: { path: { id, teamId, bindingId } } }),
  );

export const listInvitations = (id: string): Promise<TInvitation[]> =>
  request(() => ApiClient.GET('/organizations/{id}/invitations', { params: org(id) }));

export const createInvitation = (id: string, email: string, role: TBindableRole): Promise<TInvitationIssued> =>
  request(() => ApiClient.POST('/organizations/{id}/invitations', { params: org(id), body: { Email: email, Role: role } }));

const invitation = (id: string, invitationId: string) => ({ path: { id, invitationId } });

export const revokeInvitation = (id: string, invitationId: string): Promise<TInvitation> =>
  request(() => ApiClient.DELETE('/organizations/{id}/invitations/{invitationId}', { params: invitation(id, invitationId) }));

export const resendInvitation = (id: string, invitationId: string): Promise<TInvitationIssued> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/invitations/{invitationId}/resend', { params: invitation(id, invitationId) }),
  );
