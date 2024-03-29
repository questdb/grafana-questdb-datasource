import React from 'react';
import {
  DataSourcePluginOptionsEditorProps,
  onUpdateDatasourceJsonDataOption,
  onUpdateDatasourceSecureJsonDataOption, SelectableValue,
} from '@grafana/data';
import {Field, Input, SecretInput, Select, Switch} from '@grafana/ui';
import {CertificationKey} from '../components/ui/CertificationKey';
import {Components} from './../selectors';
import {PostgresTLSModes, PostgresTLSMethods, QuestDBConfig, QuestDBSecureConfig} from './../types';
import {gte} from 'semver';
import {ConfigSection, DataSourceDescription} from '@grafana/experimental';
import {config} from '@grafana/runtime';
import {Divider} from 'components/Divider';

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
  const onTlsConfigurationMethodChange = (method?: PostgresTLSMethods) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...options.jsonData,
        tlsConfigurationMethod: method,
      },
    });
  };
  const onSwitchToggle = (
    key: keyof Pick<QuestDBConfig, 'validate' | 'enableSecureSocksProxy'>,
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

  const tlsModes: Array<SelectableValue<PostgresTLSModes>> = [
    { value: PostgresTLSModes.disable, label: 'disable' },
    { value: PostgresTLSModes.require, label: 'require' },
    { value: PostgresTLSModes.verifyCA, label: 'verify-ca' },
    { value: PostgresTLSModes.verifyFull, label: 'verify-full' },
  ];

  const tlsMethods: Array<SelectableValue<PostgresTLSMethods>> = [
    { value: PostgresTLSMethods.filePath, label: 'File system path' },
    { value: PostgresTLSMethods.fileContent, label: 'Certificate content' },
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
        <Field
          label={Components.ConfigEditor.Username.label}
          description={Components.ConfigEditor.Username.tooltip}
        >
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
              onChange={onUpdateDatasourceJsonDataOption(props, 'maxOpenConnections')}
              label={Components.ConfigEditor.MaxOpenConnections.label}
              aria-label={Components.ConfigEditor.MaxOpenConnections.label}
              placeholder={Components.ConfigEditor.MaxOpenConnections.placeholder}
              defaultValue={Components.ConfigEditor.MaxOpenConnections.placeholder}
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
              onChange={onUpdateDatasourceJsonDataOption(props, 'maxIdleConnections')}
              label={Components.ConfigEditor.MaxIdleConnections.label}
              aria-label={Components.ConfigEditor.MaxIdleConnections.label}
              placeholder={Components.ConfigEditor.MaxIdleConnections.placeholder}
              defaultValue={Components.ConfigEditor.MaxIdleConnections.placeholder}
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
              onChange={onUpdateDatasourceJsonDataOption(props, 'maxConnectionLifetime')}
              label={Components.ConfigEditor.MaxConnectionLifetime.label}
              aria-label={Components.ConfigEditor.MaxConnectionLifetime.label}
              placeholder={Components.ConfigEditor.MaxConnectionLifetime.placeholder}
              defaultValue={Components.ConfigEditor.MaxConnectionLifetime.placeholder}
          />
        </Field>

        <Field label={Components.ConfigEditor.Timeout.label} description={Components.ConfigEditor.Timeout.tooltip}>
          <Input
              name="timeout"
              width={40}
              value={jsonData.timeout || ''}
              onChange={onUpdateDatasourceJsonDataOption(props, 'timeout')}
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
              onChange={onUpdateDatasourceJsonDataOption(props, 'queryTimeout')}
              label={Components.ConfigEditor.QueryTimeout.label}
              aria-label={Components.ConfigEditor.QueryTimeout.label}
              placeholder={Components.ConfigEditor.QueryTimeout.placeholder}
              defaultValue={Components.ConfigEditor.QueryTimeout.placeholder}
              type="number"
          />
        </Field>

        {config.featureToggles['secureSocksDSProxyEnabled'] && gte(config.buildInfo.version, '10.0.0') && (
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
        )}

      </ConfigSection>

      <Divider />
      <ConfigSection title="TLS / SSL Settings">
        <Field label={Components.ConfigEditor.TlsMode.label} description={Components.ConfigEditor.TlsMode.tooltip}>
          <Select
              id="tlsMode"
              width={40}
              className="gf-form"
              options={tlsModes}
              value={jsonData.tlsMode || PostgresTLSModes.verifyFull}
              onChange={(e) =>  onTlsModeChange(e.value)}
              placeholder={Components.ConfigEditor.TlsMode.placeholder}
          />
        </Field>

        {jsonData.tlsMode !== PostgresTLSModes.disable ? (
            <>
            <Field label={Components.ConfigEditor.TlsMethod.label}  description={Components.ConfigEditor.TlsMethod.tooltip}>
              <Select
                  options={tlsMethods}
                  value={jsonData.tlsConfigurationMethod || PostgresTLSMethods.filePath}
                  onChange={e=>onTlsConfigurationMethodChange(e.value)}
                  placeholder={Components.ConfigEditor.TlsMethod.placeholder}
                  width={40}
              />
            </Field>

              {jsonData.tlsConfigurationMethod === PostgresTLSMethods.fileContent ? (
                  <>
                    <CertificationKey
                        hasCert={!!hasTLSCACert}
                        onChange={(e) => onCertificateChangeFactory('tlsCACert', e.currentTarget.value)}
                        placeholder={Components.ConfigEditor.TLSCACert.placeholder}
                        label={Components.ConfigEditor.TLSCACert.label}
                        onClick={() => onResetClickFactory('tlsCACert')}
                    />
                    { false && (
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
              ) : (
                  <>
                    <Field label={Components.ConfigEditor.TLSCACertFile.label}
                           description={Components.ConfigEditor.TLSCACertFile.placeholder}>
                      <Input
                          value={jsonData.tlsCACertFile || ''}
                          onChange={onUpdateDatasourceJsonDataOption(props, 'tlsCACertFile')}
                          width={40}
                          placeholder={Components.ConfigEditor.TLSCACertFile.placeholder}
                      />
                    </Field>
                    { false && (
                        <>
                          <Field label={Components.ConfigEditor.TLSClientCertFile.label}
                                 description={Components.ConfigEditor.TLSClientCertFile.placeholder} >
                            <Input
                                value={jsonData.tlsClientCertFile || ''}
                                onChange={onUpdateDatasourceJsonDataOption(props, 'tlsClientCertFile')}
                                width={40}
                            />
                          </Field>
                          <Field label={Components.ConfigEditor.TLSClientKeyFile.label}
                                 description={Components.ConfigEditor.TLSClientKeyFile.placeholder}>
                            <Input
                                value={jsonData.tlsClientKeyFile || ''}
                                onChange={onUpdateDatasourceJsonDataOption(props, 'tlsClientKeyFile')}
                                width={40}
                            />
                          </Field>
                        </>
                    )}
                  </>
              )}
          </>
        ) : null}
      </ConfigSection>
      <Divider />
    </>
  );
};
