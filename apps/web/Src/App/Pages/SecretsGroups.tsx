import { PageGroup } from '@/App/Layout/PageGroup';
import { useSlide } from '@/App/Navigation/useSlide';
import { registryEditSlide, secretEditSlide, variableEditSlide } from '@/App/Panels';
import { useT } from '@/I18n/useT';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { RegistrySection } from '@/Modules/Secrets/Components/RegistrySection';
import { SecretsSection } from '@/Modules/Secrets/Components/SecretsSection';
import { VariablesSection } from '@/Modules/Secrets/Components/VariablesSection';

export interface ISecretsGroupsProps {
  scope: ISecretScope;
  writable: boolean;
}

export const SecretsGroups = ({ scope, writable }: ISecretsGroupsProps) => {
  const t = useT();
  const { open } = useSlide();

  return (
    <>
      <PageGroup title={t('secrets.group.secrets')} lead={t('secrets.group.secrets.lead')} icon="key">
        <SecretsSection
          scope={scope}
          writable={writable}
          onNew={() => {
            open('secret-new');
          }}
          onEdit={(name) => {
            open(secretEditSlide(name));
          }}
        />
      </PageGroup>
      <PageGroup title={t('secrets.group.variables')} lead={t('secrets.group.variables.lead')} icon="terminal">
        <VariablesSection
          scope={scope}
          writable={writable}
          onNew={() => {
            open('variable-new');
          }}
          onEdit={(name) => {
            open(variableEditSlide(name));
          }}
        />
      </PageGroup>
    </>
  );
};

export const RegistryGroup = ({ scope, writable }: ISecretsGroupsProps) => {
  const t = useT();
  const { open } = useSlide();

  return (
    <PageGroup title={t('secrets.group.registry')} lead={t('secrets.group.registry.lead')} icon="server">
      <RegistrySection
        scope={scope}
        writable={writable}
        onNew={() => {
          open('registry-new');
        }}
        onEdit={(credentialId) => {
          open(registryEditSlide(credentialId));
        }}
      />
    </PageGroup>
  );
};
