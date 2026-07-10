import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
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

  describe('service account routing', () => {
    const lastJsonData = (props: ReturnType<typeof mockConfigEditorProps>) => {
      const calls = (props.onOptionsChange as jest.Mock).mock.calls;
      return calls[calls.length - 1][0].jsonData;
    };

    it('renders the routing toggle and hides details when disabled', () => {
      render(<ConfigEditor {...mockConfigEditorProps()} />);
      expect(screen.getByText(Components.ConfigEditor.ServiceAccountRouting.label)).toBeInTheDocument();
      expect(
        screen.queryByPlaceholderText(Components.ConfigEditor.DefaultServiceAccount.placeholder)
      ).not.toBeInTheDocument();
    });

    it('shows default service account and mappings when enabled', () => {
      render(
        <ConfigEditor
          {...mockConfigEditorProps({
            serviceAccountRoutingEnabled: true,
            defaultServiceAccount: 'sa_default',
            serviceAccountMappings: [{ grafanaUser: 'john', serviceAccount: 'sa_analysts' }],
          })}
        />
      );
      expect(
        screen.getByPlaceholderText(Components.ConfigEditor.DefaultServiceAccount.placeholder)
      ).toBeInTheDocument();
      expect(screen.getByDisplayValue('sa_default')).toBeInTheDocument();
      expect(screen.getByDisplayValue('john')).toBeInTheDocument();
      expect(screen.getByDisplayValue('sa_analysts')).toBeInTheDocument();
    });

    it('toggling the switch enables routing', () => {
      const props = mockConfigEditorProps();
      render(<ConfigEditor {...props} />);
      fireEvent.click(screen.getByLabelText(Components.ConfigEditor.ServiceAccountRouting.label));
      expect(lastJsonData(props).serviceAccountRoutingEnabled).toBe(true);
    });

    it('disabling routing also clears Forward OAuth Identity', () => {
      // oauthPassThru is only reachable from inside the routing block, so turning routing off
      // must clear it too — otherwise it is stranded on with no UI to disable it.
      const props = mockConfigEditorProps({ serviceAccountRoutingEnabled: true, oauthPassThru: true });
      render(<ConfigEditor {...props} />);
      fireEvent.click(screen.getByLabelText(Components.ConfigEditor.ServiceAccountRouting.label));
      const jd = lastJsonData(props);
      expect(jd.serviceAccountRoutingEnabled).toBe(false);
      expect(jd.oauthPassThru).toBe(false);
    });

    it('hides the Forward OAuth Identity switch until routing is enabled', () => {
      render(<ConfigEditor {...mockConfigEditorProps()} />);
      expect(
        screen.queryByLabelText(Components.ConfigEditor.ForwardOAuthIdentity.label)
      ).not.toBeInTheDocument();
    });

    it('toggling Forward OAuth Identity sets oauthPassThru', () => {
      const props = mockConfigEditorProps({ serviceAccountRoutingEnabled: true });
      render(<ConfigEditor {...props} />);
      fireEvent.click(screen.getByLabelText(Components.ConfigEditor.ForwardOAuthIdentity.label));
      expect(lastJsonData(props).oauthPassThru).toBe(true);
    });

    it('adds a mapping row', () => {
      const props = mockConfigEditorProps({ serviceAccountRoutingEnabled: true });
      render(<ConfigEditor {...props} />);
      fireEvent.click(screen.getByText(Components.ConfigEditor.ServiceAccountMappings.addLabel));
      expect(lastJsonData(props).serviceAccountMappings).toEqual([{ grafanaUser: '', serviceAccount: '' }]);
    });

    it('updates a mapping field', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountMappings: [{ grafanaUser: '', serviceAccount: '' }],
      });
      render(<ConfigEditor {...props} />);
      fireEvent.change(screen.getByPlaceholderText(Components.ConfigEditor.ServiceAccountMappings.grafanaUserPlaceholder), {
        target: { value: 'alice' },
      });
      expect(lastJsonData(props).serviceAccountMappings).toEqual([{ grafanaUser: 'alice', serviceAccount: '' }]);
    });

    it('removes a mapping row', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountMappings: [{ grafanaUser: 'john', serviceAccount: 'sa_analysts' }],
      });
      render(<ConfigEditor {...props} />);
      fireEvent.click(
        screen.getByRole('button', { name: `${Components.ConfigEditor.ServiceAccountMappings.removeLabel} 1` })
      );
      expect(lastJsonData(props).serviceAccountMappings).toEqual([]);
    });

    it('editing the default service account updates jsonData', () => {
      const props = mockConfigEditorProps({ serviceAccountRoutingEnabled: true });
      render(<ConfigEditor {...props} />);
      fireEvent.change(screen.getByPlaceholderText(Components.ConfigEditor.DefaultServiceAccount.placeholder), {
        target: { value: 'sa_default' },
      });
      expect(lastJsonData(props).defaultServiceAccount).toBe('sa_default');
    });

    it('removes the correct row among several', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountMappings: [
          { grafanaUser: 'a', serviceAccount: 'sa_a' },
          { grafanaUser: 'b', serviceAccount: 'sa_b' },
          { grafanaUser: 'c', serviceAccount: 'sa_c' },
        ],
      });
      render(<ConfigEditor {...props} />);
      fireEvent.click(
        screen.getByRole('button', { name: `${Components.ConfigEditor.ServiceAccountMappings.removeLabel} 2` })
      );
      expect(lastJsonData(props).serviceAccountMappings).toEqual([
        { grafanaUser: 'a', serviceAccount: 'sa_a' },
        { grafanaUser: 'c', serviceAccount: 'sa_c' },
      ]);
    });

    it('updates the correct row among several', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountMappings: [
          { grafanaUser: 'a', serviceAccount: 'sa_a' },
          { grafanaUser: 'b', serviceAccount: 'sa_b' },
        ],
      });
      render(<ConfigEditor {...props} />);
      const saInputs = screen.getAllByPlaceholderText(
        Components.ConfigEditor.ServiceAccountMappings.serviceAccountPlaceholder
      );
      fireEvent.change(saInputs[1], { target: { value: 'sa_b2' } });
      expect(lastJsonData(props).serviceAccountMappings).toEqual([
        { grafanaUser: 'a', serviceAccount: 'sa_a' },
        { grafanaUser: 'b', serviceAccount: 'sa_b2' },
      ]);
    });

    it('shows group mappings and groups claim when enabled', () => {
      render(
        <ConfigEditor
          {...mockConfigEditorProps({
            serviceAccountRoutingEnabled: true,
            groupsClaim: 'myclaim',
            serviceAccountGroupMappings: [{ group: 'Analysts', serviceAccount: 'sa_grp' }],
          })}
        />
      );
      expect(
        screen.getByPlaceholderText(Components.ConfigEditor.ServiceAccountGroupMappings.groupPlaceholder)
      ).toBeInTheDocument();
      expect(screen.getByDisplayValue('Analysts')).toBeInTheDocument();
      expect(screen.getByDisplayValue('sa_grp')).toBeInTheDocument();
      expect(screen.getByDisplayValue('myclaim')).toBeInTheDocument();
    });

    it('adds a group mapping row', () => {
      const props = mockConfigEditorProps({ serviceAccountRoutingEnabled: true });
      render(<ConfigEditor {...props} />);
      fireEvent.click(screen.getByText(Components.ConfigEditor.ServiceAccountGroupMappings.addLabel));
      expect(lastJsonData(props).serviceAccountGroupMappings).toEqual([{ group: '', serviceAccount: '' }]);
    });

    it('updates a group mapping field', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountGroupMappings: [{ group: '', serviceAccount: '' }],
      });
      render(<ConfigEditor {...props} />);
      fireEvent.change(
        screen.getByPlaceholderText(Components.ConfigEditor.ServiceAccountGroupMappings.groupPlaceholder),
        { target: { value: 'Engineering' } }
      );
      expect(lastJsonData(props).serviceAccountGroupMappings).toEqual([{ group: 'Engineering', serviceAccount: '' }]);
    });

    it('removes a group mapping row', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountGroupMappings: [{ group: 'Analysts', serviceAccount: 'sa_grp' }],
      });
      render(<ConfigEditor {...props} />);
      fireEvent.click(
        screen.getByRole('button', { name: `${Components.ConfigEditor.ServiceAccountGroupMappings.removeLabel} 1` })
      );
      expect(lastJsonData(props).serviceAccountGroupMappings).toEqual([]);
    });

    it('editing the groups claim updates jsonData', () => {
      const props = mockConfigEditorProps({ serviceAccountRoutingEnabled: true });
      render(<ConfigEditor {...props} />);
      fireEvent.change(screen.getByPlaceholderText(Components.ConfigEditor.GroupsClaim.placeholder), {
        target: { value: 'roles' },
      });
      expect(lastJsonData(props).groupsClaim).toBe('roles');
    });

    it('removes the correct group row among several', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountGroupMappings: [
          { group: 'A', serviceAccount: 'sa_a' },
          { group: 'B', serviceAccount: 'sa_b' },
          { group: 'C', serviceAccount: 'sa_c' },
        ],
      });
      render(<ConfigEditor {...props} />);
      fireEvent.click(
        screen.getByRole('button', { name: `${Components.ConfigEditor.ServiceAccountGroupMappings.removeLabel} 2` })
      );
      expect(lastJsonData(props).serviceAccountGroupMappings).toEqual([
        { group: 'A', serviceAccount: 'sa_a' },
        { group: 'C', serviceAccount: 'sa_c' },
      ]);
    });

    it('updates the correct group row among several', () => {
      const props = mockConfigEditorProps({
        serviceAccountRoutingEnabled: true,
        serviceAccountGroupMappings: [
          { group: 'A', serviceAccount: 'sa_a' },
          { group: 'B', serviceAccount: 'sa_b' },
        ],
      });
      render(<ConfigEditor {...props} />);
      // Query by the group-specific service-account aria-label so this stays unambiguous even
      // when the user-mapping list (which shares the "Service account" placeholder) also renders.
      const groupSaLabel = `${Components.ConfigEditor.ServiceAccountGroupMappings.groupPlaceholder} ${Components.ConfigEditor.ServiceAccountGroupMappings.serviceAccountPlaceholder} 2`;
      fireEvent.change(screen.getByLabelText(groupSaLabel), { target: { value: 'sa_b2' } });
      expect(lastJsonData(props).serviceAccountGroupMappings).toEqual([
        { group: 'A', serviceAccount: 'sa_a' },
        { group: 'B', serviceAccount: 'sa_b2' },
      ]);
    });

    it('warns when group mappings exist but Forward OAuth Identity is off', () => {
      render(
        <ConfigEditor
          {...mockConfigEditorProps({
            serviceAccountRoutingEnabled: true,
            serviceAccountGroupMappings: [{ group: 'Analysts', serviceAccount: 'sa_grp' }],
          })}
        />
      );
      expect(
        screen.getByText(Components.ConfigEditor.ServiceAccountGroupMappings.forwardOAuthWarning)
      ).toBeInTheDocument();
    });

    it('hides the warning once Forward OAuth Identity is enabled', () => {
      render(
        <ConfigEditor
          {...mockConfigEditorProps({
            serviceAccountRoutingEnabled: true,
            oauthPassThru: true,
            serviceAccountGroupMappings: [{ group: 'Analysts', serviceAccount: 'sa_grp' }],
          })}
        />
      );
      expect(
        screen.queryByText(Components.ConfigEditor.ServiceAccountGroupMappings.forwardOAuthWarning)
      ).not.toBeInTheDocument();
    });

    it('does not warn when there are no group mappings', () => {
      render(<ConfigEditor {...mockConfigEditorProps({ serviceAccountRoutingEnabled: true })} />);
      expect(
        screen.queryByText(Components.ConfigEditor.ServiceAccountGroupMappings.forwardOAuthWarning)
      ).not.toBeInTheDocument();
    });
  });
});
