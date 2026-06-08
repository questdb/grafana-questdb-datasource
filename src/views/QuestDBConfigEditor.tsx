import React from 'react';
import {
  DataSourcePluginOptionsEditorProps,
  onUpdateDatasourceJsonDataOption,
  onUpdateDatasourceSecureJsonDataOption,
  SelectableValue,
} from '@grafana/data';
import { Field, Input, SecretInput, Select, Switch } from '@grafana/ui';
import { CertificationKey } from '../components/ui/CertificationKey';
import { MappingList } from '../components/ui/MappingList';
import { Components } from './../selectors';
import {
  PostgresTLSModes,
  QuestDBConfig,
  QuestDBSecureConfig,
  ServiceAccountGroupMapping,
  ServiceAccountMapping,
} from './../types';
import { gte } from 'semver';
import { ConfigSection, DataSourceDescription } from '@grafana/experimental';
import { config } from '@grafana/runtime';
import { Divider } from 'components/Divider';

export interface Props extends DataSourcePluginOptionsEditorProps<QuestDBConfig> {}

export const ConfigEditor: React.FC<Props> = (props) => {
  const { options, onOptionsChange } = props;
  const { jsonData, secureJsonFields } = options;
  const secureJsonData = (options.secureJsonData || {}) as QuestDBSecureConfig;
  const hasTLSCACert = secureJsonFields && secureJsonFields.tlsCACert;
  const hasTLSClientCert = secureJsonFields && secureJsonFields.tlsClientCert;
  const hasTLSClientKey = secureJsonFields && secureJsonFields.tlsClientKey;
  const onPortChange = (port: string) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        port: +port,
      },
    });
  };
  const onTlsModeChange = (mode?: PostgresTLSModes) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        tlsMode: mode,
      },
    });
  };
  const onSwitchToggle = (
    key: keyof Pick<QuestDBConfig, 'validate' | 'enableSecureSocksProxy' | 'serviceAccountRoutingEnabled'>,
    value: boolean
  ) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        [key]: value,
      },
    });
  };

  const mappings = jsonData.serviceAccountMappings ?? [];
  const onMappingsChange = (next: ServiceAccountMapping[]) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        serviceAccountMappings: next,
      },
    });
  };

  const groupMappings = jsonData.serviceAccountGroupMappings ?? [];
  const onGroupMappingsChange = (next: ServiceAccountGroupMapping[]) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        serviceAccountGroupMappings: next,
      },
    });
  };

  const onCertificateChangeFactory = (key: keyof Omit<QuestDBSecureConfig, 'password'>, value: string) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        [key]: value,
      },
    });
  };
  const onResetClickFactory = (key: keyof Omit<QuestDBSecureConfig, 'password'>) => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        [key]: false,
      },
      secureJsonData: {
        ...secureJsonData,
        [key]: '',
      },
    });
  };
  const onResetPassword = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...options.secureJsonFields,
        password: false,
      },
      secureJsonData: {
        ...options.secureJsonData,
        password: '',
      },
    });
  };

  const onUpdateNumberOption = (
    key: keyof Pick<
      QuestDBConfig,
      'timeout' | 'queryTimeout' | 'maxConnectionLifetime' | 'maxIdleConnections' | 'maxOpenConnections'
    >,
    value: string
  ) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        [key]: value ? +value : undefined,
      },
    });
  };

  const tlsModes: Array<SelectableValue<PostgresTLSModes>> = [
    { value: PostgresTLSModes.disable, label: 'disable' },
    { value: PostgresTLSModes.require, label: 'require' },
    { value: PostgresTLSModes.verifyCA, label: 'verify-ca' },
    { value: PostgresTLSModes.verifyFull, label: 'verify-full' },
  ];

  return (
    <>
      <DataSourceDescription
        dataSourceName="QuestDB"
        docsLink="https://github.com/questdb/grafana-questdb-datasource/"
        hasRequiredFields
      />
      <Divider />
      <ConfigSection title="Server">
        <Field
          required
          label={Components.ConfigEditor.ServerAddress.label}
          description={Components.ConfigEditor.ServerAddress.tooltip}
        >
          <Input
            name="server"
            width={40}
            value={jsonData.server || ''}
            onChange={onUpdateDatasourceJsonDataOption(props, 'server')}
            label={Components.ConfigEditor.ServerAddress.label}
            aria-label={Components.ConfigEditor.ServerAddress.label}
            placeholder={Components.ConfigEditor.ServerAddress.placeholder}
          />
        </Field>
        <Field
          required
          label={Components.ConfigEditor.ServerPort.label}
          description={Components.ConfigEditor.ServerPort.tooltip}
        >
          <Input
            name="port"
            width={40}
            type="number"
            value={jsonData.port || ''}
            onChange={(e) => onPortChange(e.currentTarget.value)}
            label={Components.ConfigEditor.ServerPort.label}
            aria-label={Components.ConfigEditor.ServerPort.label}
            placeholder={Components.ConfigEditor.ServerPort.placeholder}
          />
        </Field>
      </ConfigSection>

      <Divider />
      <ConfigSection title="Credentials">
        <Field label={Components.ConfigEditor.Username.label} description={Components.ConfigEditor.Username.tooltip}>
          <Input
            name="user"
            width={40}
            value={jsonData.username || ''}
            onChange={onUpdateDatasourceJsonDataOption(props, 'username')}
            label={Components.ConfigEditor.Username.label}
            aria-label={Components.ConfigEditor.Username.label}
            placeholder={Components.ConfigEditor.Username.placeholder}
          />
        </Field>
        <Field label={Components.ConfigEditor.Password.label} description={Components.ConfigEditor.Password.tooltip}>
          <SecretInput
            name="pwd"
            width={40}
            label={Components.ConfigEditor.Password.label}
            aria-label={Components.ConfigEditor.Password.label}
            placeholder={Components.ConfigEditor.Password.placeholder}
            value={secureJsonData.password || ''}
            isConfigured={(secureJsonFields && secureJsonFields.password) as boolean}
            onReset={onResetPassword}
            onChange={onUpdateDatasourceSecureJsonDataOption(props, 'password')}
          />
        </Field>
      </ConfigSection>

      <Divider />
      <ConfigSection title="Connection limits">
        <Field
          label={Components.ConfigEditor.MaxOpenConnections.label}
          description={Components.ConfigEditor.MaxOpenConnections.tooltip}
        >
          <Input
            name="maxOpenConnections"
            width={40}
            value={jsonData.maxOpenConnections || ''}
            onChange={(e) => onUpdateNumberOption('maxOpenConnections', e.currentTarget.value)}
            label={Components.ConfigEditor.MaxOpenConnections.label}
            aria-label={Components.ConfigEditor.MaxOpenConnections.label}
            placeholder={Components.ConfigEditor.MaxOpenConnections.placeholder}
            defaultValue={Components.ConfigEditor.MaxOpenConnections.placeholder}
            type="number"
          />
        </Field>

        <Field
          label={Components.ConfigEditor.MaxIdleConnections.label}
          description={Components.ConfigEditor.MaxIdleConnections.tooltip}
        >
          <Input
            name="maxIdleConnections"
            width={40}
            value={jsonData.maxIdleConnections || ''}
            onChange={(e) => onUpdateNumberOption('maxIdleConnections', e.currentTarget.value)}
            label={Components.ConfigEditor.MaxIdleConnections.label}
            aria-label={Components.ConfigEditor.MaxIdleConnections.label}
            placeholder={Components.ConfigEditor.MaxIdleConnections.placeholder}
            defaultValue={Components.ConfigEditor.MaxIdleConnections.placeholder}
            type="number"
          />
        </Field>

        <Field
          label={Components.ConfigEditor.MaxConnectionLifetime.label}
          description={Components.ConfigEditor.MaxConnectionLifetime.tooltip}
        >
          <Input
            name="maxConnectionLifetime"
            width={40}
            value={jsonData.maxConnectionLifetime || ''}
            onChange={(e) => onUpdateNumberOption('maxConnectionLifetime', e.currentTarget.value)}
            label={Components.ConfigEditor.MaxConnectionLifetime.label}
            aria-label={Components.ConfigEditor.MaxConnectionLifetime.label}
            placeholder={Components.ConfigEditor.MaxConnectionLifetime.placeholder}
            defaultValue={Components.ConfigEditor.MaxConnectionLifetime.placeholder}
            type="number"
          />
        </Field>

        <Field label={Components.ConfigEditor.Timeout.label} description={Components.ConfigEditor.Timeout.tooltip}>
          <Input
            name="timeout"
            width={40}
            value={jsonData.timeout || ''}
            onChange={(e) => onUpdateNumberOption('timeout', e.currentTarget.value)}
            label={Components.ConfigEditor.Timeout.label}
            aria-label={Components.ConfigEditor.Timeout.label}
            placeholder={Components.ConfigEditor.Timeout.placeholder}
            defaultValue={Components.ConfigEditor.Timeout.placeholder}
            type="number"
          />
        </Field>
        <Field
          label={Components.ConfigEditor.QueryTimeout.label}
          description={Components.ConfigEditor.QueryTimeout.tooltip}
        >
          <Input
            name="queryTimeout"
            width={40}
            value={jsonData.queryTimeout || ''}
            onChange={(e) => onUpdateNumberOption('queryTimeout', e.currentTarget.value)}
            label={Components.ConfigEditor.QueryTimeout.label}
            aria-label={Components.ConfigEditor.QueryTimeout.label}
            placeholder={Components.ConfigEditor.QueryTimeout.placeholder}
            defaultValue={Components.ConfigEditor.QueryTimeout.placeholder}
            type="number"
          />
        </Field>
        <Field
          label={Components.ConfigEditor.MinInterval.label}
          description={Components.ConfigEditor.MinInterval.tooltip}
        >
          <Input
            name="timeInterval"
            width={40}
            value={jsonData.timeInterval || ''}
            onChange={onUpdateDatasourceJsonDataOption(props, 'timeInterval')}
            label={Components.ConfigEditor.MinInterval.label}
            aria-label={Components.ConfigEditor.MinInterval.label}
          />
        </Field>
      </ConfigSection>

      {config.secureSocksDSProxyEnabled && gte(config.buildInfo.version, '10.0.0') && (
        <>
          <Divider />

          <ConfigSection title="Proxy">
            <Field
              label={Components.ConfigEditor.SecureSocksProxy.label}
              description={Components.ConfigEditor.SecureSocksProxy.tooltip}
            >
              <Switch
                className="gf-form"
                value={jsonData.enableSecureSocksProxy || false}
                onChange={(e) => onSwitchToggle('enableSecureSocksProxy', e.currentTarget.checked)}
              />
            </Field>
          </ConfigSection>
        </>
      )}

      <Divider />
      <ConfigSection title="TLS / SSL Settings">
        <Field label={Components.ConfigEditor.TlsMode.label} description={Components.ConfigEditor.TlsMode.tooltip}>
          <Select
            id="tlsMode"
            width={40}
            className="gf-form"
            options={tlsModes}
            value={jsonData.tlsMode}
            onChange={(e) => onTlsModeChange(e.value)}
            placeholder={Components.ConfigEditor.TlsMode.placeholder}
          />
        </Field>

        {jsonData.tlsMode && jsonData.tlsMode !== PostgresTLSModes.disable ? (
          <>
            <>
              <CertificationKey
                hasCert={!!hasTLSCACert}
                onChange={(e) => onCertificateChangeFactory('tlsCACert', e.currentTarget.value)}
                placeholder={Components.ConfigEditor.TLSCACert.placeholder}
                label={Components.ConfigEditor.TLSCACert.label}
                tooltip={Components.ConfigEditor.TLSCACert.tooltip}
                onClick={() => onResetClickFactory('tlsCACert')}
              />
              {false && (
                <>
                  <CertificationKey
                    hasCert={!!hasTLSClientCert}
                    onChange={(e) => onCertificateChangeFactory('tlsClientCert', e.currentTarget.value)}
                    placeholder={Components.ConfigEditor.TLSClientCert.placeholder}
                    label={Components.ConfigEditor.TLSClientCert.label}
                    onClick={() => onResetClickFactory('tlsClientCert')}
                  />
                  <CertificationKey
                    hasCert={!!hasTLSClientKey}
                    placeholder={Components.ConfigEditor.TLSClientKey.placeholder}
                    label={Components.ConfigEditor.TLSClientKey.label}
                    onChange={(e) => onCertificateChangeFactory('tlsClientKey', e.currentTarget.value)}
                    onClick={() => onResetClickFactory('tlsClientKey')}
                  />
                </>
              )}
            </>
          </>
        ) : null}
      </ConfigSection>

      <Divider />
      <ConfigSection title="Per-user service accounts">
        <Field
          label={Components.ConfigEditor.ServiceAccountRouting.label}
          description={Components.ConfigEditor.ServiceAccountRouting.tooltip}
        >
          <Switch
            className="gf-form"
            aria-label={Components.ConfigEditor.ServiceAccountRouting.label}
            value={jsonData.serviceAccountRoutingEnabled || false}
            onChange={(e) => onSwitchToggle('serviceAccountRoutingEnabled', e.currentTarget.checked)}
          />
        </Field>

        {jsonData.serviceAccountRoutingEnabled && (
          <>
            <Field
              label={Components.ConfigEditor.DefaultServiceAccount.label}
              description={Components.ConfigEditor.DefaultServiceAccount.tooltip}
            >
              <Input
                name="defaultServiceAccount"
                width={40}
                value={jsonData.defaultServiceAccount || ''}
                onChange={onUpdateDatasourceJsonDataOption(props, 'defaultServiceAccount')}
                label={Components.ConfigEditor.DefaultServiceAccount.label}
                aria-label={Components.ConfigEditor.DefaultServiceAccount.label}
                placeholder={Components.ConfigEditor.DefaultServiceAccount.placeholder}
              />
            </Field>

            <Field
              label={Components.ConfigEditor.ServiceAccountMappings.label}
              description={Components.ConfigEditor.ServiceAccountMappings.tooltip}
            >
              <MappingList<ServiceAccountMapping>
                items={mappings}
                newRow={() => ({ grafanaUser: '', serviceAccount: '' })}
                onChange={onMappingsChange}
                addLabel={Components.ConfigEditor.ServiceAccountMappings.addLabel}
                removeLabel={Components.ConfigEditor.ServiceAccountMappings.removeLabel}
                columns={[
                  {
                    field: 'grafanaUser',
                    placeholder: Components.ConfigEditor.ServiceAccountMappings.grafanaUserPlaceholder,
                    ariaLabel: Components.ConfigEditor.ServiceAccountMappings.grafanaUserPlaceholder,
                  },
                  {
                    field: 'serviceAccount',
                    placeholder: Components.ConfigEditor.ServiceAccountMappings.serviceAccountPlaceholder,
                    ariaLabel: Components.ConfigEditor.ServiceAccountMappings.serviceAccountPlaceholder,
                  },
                ]}
              />
            </Field>

            <Field
              label={Components.ConfigEditor.GroupsClaim.label}
              description={Components.ConfigEditor.GroupsClaim.tooltip}
            >
              <Input
                name="groupsClaim"
                width={40}
                value={jsonData.groupsClaim || ''}
                onChange={onUpdateDatasourceJsonDataOption(props, 'groupsClaim')}
                label={Components.ConfigEditor.GroupsClaim.label}
                aria-label={Components.ConfigEditor.GroupsClaim.label}
                placeholder={Components.ConfigEditor.GroupsClaim.placeholder}
              />
            </Field>

            <Field
              label={Components.ConfigEditor.ServiceAccountGroupMappings.label}
              description={Components.ConfigEditor.ServiceAccountGroupMappings.tooltip}
            >
              <MappingList<ServiceAccountGroupMapping>
                items={groupMappings}
                newRow={() => ({ group: '', serviceAccount: '' })}
                onChange={onGroupMappingsChange}
                addLabel={Components.ConfigEditor.ServiceAccountGroupMappings.addLabel}
                removeLabel={Components.ConfigEditor.ServiceAccountGroupMappings.removeLabel}
                columns={[
                  {
                    field: 'group',
                    placeholder: Components.ConfigEditor.ServiceAccountGroupMappings.groupPlaceholder,
                    ariaLabel: Components.ConfigEditor.ServiceAccountGroupMappings.groupPlaceholder,
                  },
                  {
                    field: 'serviceAccount',
                    placeholder: Components.ConfigEditor.ServiceAccountGroupMappings.serviceAccountPlaceholder,
                    ariaLabel: `${Components.ConfigEditor.ServiceAccountGroupMappings.groupPlaceholder} ${Components.ConfigEditor.ServiceAccountGroupMappings.serviceAccountPlaceholder}`,
                  },
                ]}
              />
            </Field>
          </>
        )}
      </ConfigSection>
      <Divider />
    </>
  );
};
