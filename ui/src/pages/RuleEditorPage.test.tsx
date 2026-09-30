import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RuleEditorPage } from './RuleEditorPage';

const mockNavigate = jest.fn();
const mockFetchRule = jest.fn();
const mockSetCurrentRule = jest.fn();
const mockUpdateCurrentRule = jest.fn();
const mockUpdateRule = jest.fn();
const mockDeployRule = jest.fn();
const mockUndeployRule = jest.fn();
const mockUndo = jest.fn();
const mockRedo = jest.fn();

const ruleFixture = {
  id: 'rule-1',
  name: 'Test Rule',
  status: 'draft',
  description: '',
  debug_mode: false,
  source: null,
  metadata: {},
  nodes: [
    {
      id: 'node-1',
      type: 'source',
      name: 'node-1',
      plugin: 'source-mqtt',
      config: {},
      wires: [],
      metadata: { position: { x: 0, y: 0 } },
    },
  ],
};

jest.mock('react-router-dom', () => ({
  useParams: () => ({ id: 'rule-1' }),
  useNavigate: () => mockNavigate,
}));

jest.mock('../components/FlowCanvas', () => ({
  FlowCanvas: ({ onNodeClick }: { onNodeClick?: (nodeId: string | null) => void }) => (
    <button onClick={() => onNodeClick?.('node-1')}>open node</button>
  ),
}));

jest.mock('../components/PropertiesPanel', () => ({
  PropertiesPanel: ({ nodeId }: { nodeId: string | null }) => <div>properties for {nodeId}</div>,
}));

jest.mock('../components/NodePalette', () => ({
  NodePalette: () => <div>palette</div>,
}));

jest.mock('../components/DryRunConsole', () => ({
  DryRunConsole: () => <div>dry-run</div>,
}));

jest.mock('../components/ConsolePage', () => ({
  SegmentTabs: () => <div>tabs</div>,
}));

jest.mock('../components/FeedbackProvider', () => ({
  useFeedback: () => ({
    warning: jest.fn(),
    success: jest.fn(),
    error: jest.fn(),
    confirm: jest.fn().mockResolvedValue(true),
  }),
}));

jest.mock('../store/ruleStore', () => {
  const store = Object.assign(jest.fn(() => ({
    currentRule: ruleFixture,
    fetchRule: mockFetchRule,
    setCurrentRule: mockSetCurrentRule,
    updateCurrentRule: mockUpdateCurrentRule,
    updateRule: mockUpdateRule,
    deployRule: mockDeployRule,
    undeployRule: mockUndeployRule,
  })), {
    getState: jest.fn(() => ({
      currentRule: ruleFixture,
      createRule: jest.fn(),
    })),
    temporal: {
      getState: () => ({
        undo: mockUndo,
        redo: mockRedo,
        pastStates: [],
        futureStates: [],
      }),
    },
  });

  return {
    useRuleStore: store,
  };
});

type MockedRuleStoreHook = jest.Mock & {
  getState: jest.Mock;
  temporal: {
    getState: jest.Mock;
  };
};

const { useRuleStore } = jest.requireMock('../store/ruleStore') as {
  useRuleStore: MockedRuleStoreHook;
};

describe('RuleEditorPage node details drawer', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useRuleStore.mockImplementation(() => ({
      currentRule: ruleFixture,
      fetchRule: mockFetchRule,
      setCurrentRule: mockSetCurrentRule,
      updateCurrentRule: mockUpdateCurrentRule,
      updateRule: mockUpdateRule,
      deployRule: mockDeployRule,
      undeployRule: mockUndeployRule,
    }));
    useRuleStore.getState.mockReturnValue({
      currentRule: ruleFixture,
      createRule: jest.fn(),
    });
  });

  it('renders node details as an absolute overlay drawer instead of reflowing the canvas', async () => {
    render(<RuleEditorPage />);

    await userEvent.click(screen.getByRole('button', { name: 'open node' }));

    const drawer = screen.getByTestId('node-details-drawer');
    expect(drawer).toBeInTheDocument();
    expect(drawer).toHaveStyle({ position: 'absolute', right: '0px' });
    expect(screen.getByText('properties for node-1')).toBeInTheDocument();
  });
});
