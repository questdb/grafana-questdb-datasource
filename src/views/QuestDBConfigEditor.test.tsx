import React from 'react';
import { render, screen } from '@testing-library/react';
import { ConfigEditor } from './QuestDBConfigEditor';
import { mockConfigEditorProps } from '../__mocks__/ConfigEditor';
import { Components } from './../selectors';
import '@testing-library/jest-dom';
import { PostgresTLSModes } from '../types';

jest.mock('@grafana/runtime', () => {
  const original = jest.requireActual('@grafana/runtime');
  return {
    ...original,
    config: { buildInfo: { version: '10.0.0' }, secureSocksDSProxyEnabled: true, featureToggles: {} },
  };
});

describe('ConfigEditor', () => {
  it('new editor', () => {
    render(<ConfigEditor {...mockConfigEditorProps()} />);
    expect(screen.getByPlaceholderText(Components.ConfigEditor.ServerAddress.placeholder)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(Components.ConfigEditor.ServerPort.placeholder)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(Components.ConfigEditor.Username.placeholder)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(Components.ConfigEditor.Password.placeholder)).toBeInTheDocument();
    expect(screen.getAllByText(Components.ConfigEditor.TlsMode.placeholder).length).toBeGreaterThan(0);
  });
  it('with password', async () => {
    render(
      <ConfigEditor
        {...mockConfigEditorProps()}
        options={{
          ...mockConfigEditorProps().options,
          secureJsonData: { password: 'secret' },
          secureJsonFields: { password: true },
        }}
      />
    );
    expect(screen.getByPlaceholderText(Components.ConfigEditor.ServerAddress.placeholder)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(Components.ConfigEditor.ServerPort.placeholder)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(Components.ConfigEditor.Username.placeholder)).toBeInTheDocument();
    expect(screen.getByText('Reset')).toBeInTheDocument();
  });
  it('with disabled tlsMode', async () => {
    render(
      <ConfigEditor
        {...mockConfigEditorProps()}
        options={{
          ...mockConfigEditorProps().options,
          jsonData: { ...mockConfigEditorProps().options.jsonData, tlsMode: PostgresTLSModes.disable },
        }}
      />
    );
    expect(screen.queryByText(PostgresTLSModes.disable)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(Components.ConfigEditor.TLSCACert.placeholder)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(Components.ConfigEditor.TLSClientCert.placeholder)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(Components.ConfigEditor.TLSClientKey.placeholder)).not.toBeInTheDocument();
  });
  it('with verifyCA tlsMode shows CA cert field', async () => {
    render(
      <ConfigEditor
        {...mockConfigEditorProps()}
        options={{
          ...mockConfigEditorProps().options,
          jsonData: {
            ...mockConfigEditorProps().options.jsonData,
            tlsMode: PostgresTLSModes.verifyCA,
          },
        }}
      />
    );
    expect(screen.queryByText(PostgresTLSModes.verifyCA)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(Components.ConfigEditor.TLSCACert.placeholder)).toBeInTheDocument();
  });

  it('with verifyFull tlsMode shows CA cert field', async () => {
    render(
      <ConfigEditor
        {...mockConfigEditorProps()}
        options={{
          ...mockConfigEditorProps().options,
          jsonData: {
            ...mockConfigEditorProps().options.jsonData,
            tlsMode: PostgresTLSModes.verifyFull,
          },
        }}
      />
    );
    expect(screen.queryByText(PostgresTLSModes.verifyFull)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(Components.ConfigEditor.TLSCACert.placeholder)).toBeInTheDocument();
  });

  it('with additional properties', async () => {
    const jsonDataOverrides = {
      queryTimeout: 100,
      timeout: 100,
      enableSecureSocksProxy: true,
    };
    render(<ConfigEditor {...mockConfigEditorProps(jsonDataOverrides)} />);
    expect(screen.getByPlaceholderText(Components.ConfigEditor.Timeout.placeholder)).toBeInTheDocument();
    expect(screen.getByText(Components.ConfigEditor.SecureSocksProxy.label)).toBeInTheDocument();
  });
});
