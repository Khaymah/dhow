import { useState } from 'react';

import type { TSecret, TSecretVisibility } from '@/Modules/Secrets/Api/SecretsApi';

export interface ISecretDraft {
  Name: string;
  Value: string;
  Description: string;
  Pulls: boolean;
  Visibility: TSecretVisibility;
  ProjectIds: string[];
  SetName: (value: string) => void;
  SetValue: (value: string) => void;
  SetDescription: (value: string) => void;
  SetPulls: (value: boolean) => void;
  SetVisibility: (value: TSecretVisibility) => void;
  SetProjectIds: (value: string[]) => void;
}

export const useSecretDraft = (existing: TSecret | undefined): ISecretDraft => {
  const [Name, setName] = useState('');
  const [Value, setValue] = useState('');
  const [Description, setDescription] = useState(existing?.Description ?? '');
  const [Pulls, setPulls] = useState(existing?.AvailableToPullRequests ?? false);
  const [Visibility, setVisibility] = useState<TSecretVisibility>(existing?.Visibility ?? 'all_projects');
  const [ProjectIds, setProjectIds] = useState<string[]>(existing?.ProjectIds ?? []);
  return {
    Name,
    Value,
    Description,
    Pulls,
    Visibility,
    ProjectIds,
    SetName: setName,
    SetValue: setValue,
    SetDescription: setDescription,
    SetPulls: setPulls,
    SetVisibility: setVisibility,
    SetProjectIds: setProjectIds,
  };
};
