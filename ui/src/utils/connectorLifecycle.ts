import { ConnectorResourceView, PluginInfo, ResourceStatus } from '../types';

const statusLabel = (status?: ResourceStatus): string => {
  switch (status) {
    case 'healthy':
      return 'Healthy';
    case 'degraded':
      return 'Degraded';
    case 'recovering':
      return 'Recovering';
    case 'congested':
      return 'Congested';
    case 'readonly':
      return 'Readonly';
    case 'disconnected':
      return 'Disconnected';
    case 'blocked':
      return 'Blocked';
    default:
      return 'Unknown';
  }
};

export const sessionPoliciesForPlugin = (plugin?: PluginInfo | null): string[] => {
  const advertised = plugin?.connector_descriptor?.capabilities
    ?.filter((capability) => capability.startsWith('session:'))
    .map((capability) => capability.slice('session:'.length))
    .filter(Boolean) || [];
  if (advertised.length > 0) return Array.from(new Set(advertised));

  switch (plugin?.connector_descriptor?.family) {
    case 's7':
	  return ['scheduled_reuse', 'per_operation'];
    case 'fanuc':
      return ['per_operation'];
    case 'mqtt':
      return ['persistent'];
    case 'postgres':
    case 'timescaledb':
    case 'tdengine':
    case 'redis':
      return ['pooled'];
    default:
      return ['shared_duplex'];
  }
};

export const defaultConnectorSessionPolicy = (plugin?: PluginInfo | null): string =>
  sessionPoliciesForPlugin(plugin)[0];

export const effectiveConnectorSessionPolicy = (connector: ConnectorResourceView): string => {
  const family = connector.plugin_info?.connector_descriptor?.family;
	if (family === 's7') {
		if (connector.session_policy === 'per_operation') return 'per_operation';
		if (connector.session_policy === 'scheduled_reuse') return 'scheduled_reuse';
		return 'scheduled_reuse';
	}
	if (family === 'fanuc') return 'per_operation';
  return connector.session_policy || defaultConnectorSessionPolicy(connector.plugin_info);
};

export const connectorStatusLabel = (connector: ConnectorResourceView): string => {
  const family = connector.plugin_info?.connector_descriptor?.family;
  const onDemand = connector.session_policy === 'per_operation' || connector.session_policy === 'scheduled_reuse' || family === 's7' || family === 'fanuc';
  if (!onDemand) return statusLabel(connector.status?.status);
	if (connector.status?.status === 'healthy') return 'Ready';
  if (connector.status?.status === 'disconnected') return 'Unreachable';
  return statusLabel(connector.status?.status);
};
