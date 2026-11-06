import { useState } from 'react';

import { EMPTY_HOST, MAX_HOSTS, type IHostDraft, type ITargetDraft } from '@/Modules/Environments/Types/TargetDraft';

export interface ITargetForm {
  Draft: ITargetDraft;
  Patch: (patch: Partial<ITargetDraft>) => void;
  SetHost: (index: number, patch: Partial<IHostDraft>) => void;
  AddHost: () => void;
  RemoveHost: (index: number) => void;
}

export const useTargetDraft = (initial: ITargetDraft): ITargetForm => {
  const [Draft, setDraft] = useState<ITargetDraft>(initial);

  return {
    Draft,
    Patch: (patch) => {
      setDraft((current) => ({ ...current, ...patch }));
    },
    SetHost: (index, patch) => {
      setDraft((current) => ({
        ...current,
        Hosts: current.Hosts.map((host, at) => (at === index ? { ...host, ...patch } : host)),
      }));
    },
    AddHost: () => {
      setDraft((current) => (current.Hosts.length >= MAX_HOSTS ? current : { ...current, Hosts: [...current.Hosts, EMPTY_HOST] }));
    },
    RemoveHost: (index) => {
      setDraft((current) => (current.Hosts.length <= 1 ? current : { ...current, Hosts: current.Hosts.filter((_, at) => at !== index) }));
    },
  };
};
