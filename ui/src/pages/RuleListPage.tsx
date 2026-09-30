import React from 'react';
import { Alert, Button, Checkbox, Input, Select, Space, Table, type TableProps } from 'antd';
import { useNavigate } from 'react-router-dom';
import { rulesAPI, runtimeAPI } from '../services/api';
import { RuleFlow, RuleSummary } from '../types';
import { useFeedback } from '../components/FeedbackProvider';
import { ActionButton, ActionMenu, AsyncState, CompactButton, FilterBar, PageLayout, StatusTag, type ActionMenuItem, type StatusTone } from '../components/ui/ConsoleUI';

interface PageMessage {
  type: 'success' | 'error';
  text: string;
}

type RuleAction = 'deploy' | 'undeploy' | 'enable' | 'disable' | 'delete';

export const RuleListPage: React.FC = () => {
  const navigate = useNavigate();
  const fileInputRef = React.useRef<HTMLInputElement | null>(null);
  const feedback = useFeedback();

  const [rules, setRules] = React.useState<RuleSummary[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [loadError, setLoadError] = React.useState('');
  const [importing, setImporting] = React.useState(false);
  const [exporting, setExporting] = React.useState(false);
  const [searchTerm, setSearchTerm] = React.useState('');
  const [filterEnabled, setFilterEnabled] = React.useState<'all' | 'enabled' | 'disabled'>('all');
  const [pageSize, setPageSize] = React.useState(20);
  const [currentPage, setCurrentPage] = React.useState(1);
  const [selectedRuleIds, setSelectedRuleIds] = React.useState<Set<string>>(new Set());
  const [message, setMessage] = React.useState<PageMessage | null>(null);
  const [pendingRuleActions, setPendingRuleActions] = React.useState<Record<string, RuleAction>>({});
  const pendingRuleActionIDs = React.useRef<Set<string>>(new Set());

  const syncRuleList = React.useCallback((ruleList: RuleSummary[]) => {
    setRules(ruleList);
    setSelectedRuleIds((prev) => {
      const next = new Set<string>();
      const validIds = new Set(ruleList.map((rule) => rule.id));
      prev.forEach((id) => {
        if (validIds.has(id)) {
          next.add(id);
        }
      });
      return next;
    });
  }, []);

  const loadRules = React.useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const ruleList = await rulesAPI.list();
      syncRuleList(ruleList);
    } catch (error) {
      console.error('Failed to load rules:', error);
      setLoadError(error instanceof Error ? error.message : '规则服务暂时不可用。');
    } finally {
      setLoading(false);
    }
  }, [syncRuleList]);

  const patchRuleSummary = React.useCallback(
    (id: string, updater: (rule: RuleSummary) => RuleSummary) => {
      setRules((prev) => prev.map((rule) => (rule.id === id ? updater(rule) : rule)));
    },
    [],
  );

  const beginRuleAction = React.useCallback((id: string, action: RuleAction) => {
    // The ref closes the small gap before React commits disabled=true, so a
    // double click cannot open two confirmations or submit two requests.
    if (pendingRuleActionIDs.current.has(id)) {
      return false;
    }
    pendingRuleActionIDs.current.add(id);
    setPendingRuleActions((prev) => ({ ...prev, [id]: action }));
    return true;
  }, []);

  const endRuleAction = React.useCallback((id: string) => {
    pendingRuleActionIDs.current.delete(id);
    setPendingRuleActions((prev) => {
      if (!prev[id]) {
        return prev;
      }
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }, []);

  React.useEffect(() => {
    void loadRules();
  }, [loadRules]);

  const filteredRules = React.useMemo(() => {
    return [...rules]
      .filter((rule) => {
      const matchesSearch =
        rule.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
        rule.id.toLowerCase().includes(searchTerm.toLowerCase());
      const matchesFilter =
        filterEnabled === 'all' ||
        (filterEnabled === 'enabled' && rule.enabled) ||
        (filterEnabled === 'disabled' && !rule.enabled);
      return matchesSearch && matchesFilter;
      })
      .sort((left, right) => left.id.localeCompare(right.id, 'en', { sensitivity: 'base' }));
  }, [filterEnabled, rules, searchTerm]);

  const totalPages = React.useMemo(
    () => Math.max(1, Math.ceil(filteredRules.length / pageSize)),
    [filteredRules.length, pageSize],
  );

  const paginatedRules = React.useMemo(() => {
    const startIndex = (currentPage - 1) * pageSize;
    return filteredRules.slice(startIndex, startIndex + pageSize);
  }, [currentPage, filteredRules, pageSize]);

  React.useEffect(() => {
    setCurrentPage(1);
  }, [filterEnabled, pageSize, searchTerm]);

  React.useEffect(() => {
    setCurrentPage((prev) => Math.min(prev, totalPages));
  }, [totalPages]);

  const selectedCount = selectedRuleIds.size;
  const allVisibleSelected =
    paginatedRules.length > 0 && paginatedRules.every((rule) => selectedRuleIds.has(rule.id));

  const toggleRuleSelection = (ruleID: string) => {
    setSelectedRuleIds((prev) => {
      const next = new Set(prev);
      if (next.has(ruleID)) {
        next.delete(ruleID);
      } else {
        next.add(ruleID);
      }
      return next;
    });
  };

  const toggleSelectAllVisible = () => {
    setSelectedRuleIds((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        paginatedRules.forEach((rule) => next.delete(rule.id));
      } else {
        paginatedRules.forEach((rule) => next.add(rule.id));
      }
      return next;
    });
  };

  const clearSelection = () => {
    setSelectedRuleIds(new Set());
  };

  const handleEnable = async (id: string) => {
    if (!beginRuleAction(id, 'enable')) return;
    try {
      await rulesAPI.enable(id);
      await loadRules();
    } catch (error) {
      console.error('Failed to enable rule:', error);
      feedback.error('启用失败');
    } finally {
      endRuleAction(id);
    }
  };

  const handleDisable = async (id: string) => {
    if (!beginRuleAction(id, 'disable')) return;
    try {
      await rulesAPI.disable(id);
      await loadRules();
    } catch (error) {
      console.error('Failed to disable rule:', error);
      feedback.error('禁用失败');
    } finally {
      endRuleAction(id);
    }
  };

  const handleDelete = async (id: string) => {
    if (!beginRuleAction(id, 'delete')) return;
    try {
      if (!await feedback.confirm({ title: '确定要删除此规则吗？', danger: true })) return;
      await rulesAPI.delete(id);
      await loadRules();
    } catch (error) {
      console.error('Failed to delete rule:', error);
      feedback.error('删除失败');
    } finally {
      endRuleAction(id);
    }
  };

  const handleDeploy = async (id: string) => {
    if (!beginRuleAction(id, 'deploy')) return;
    try {
      if (!await feedback.confirm('确定要部署此规则吗？')) return;
      const deployedRule = await rulesAPI.deploy(id);
      patchRuleSummary(id, (rule) => ({
        ...rule,
        enabled: true,
        version: deployedRule.version ?? rule.version + 1,
        status: 'deployed',
        updated_at: new Date().toISOString(),
      }));
      await loadRules();
      feedback.success('规则已部署');
    } catch (error: any) {
      console.error('Failed to deploy rule:', error);
      await loadRules();

      let errorMessage = '部署失败';
      if (error.response?.data?.details) {
        const details = error.response.data.details;
        if (Array.isArray(details)) {
          const errorList = details.map((err: any) => {
            if (err.node_id) {
              return `节点 ${err.node_id}: ${err.message}`;
            }
            return err.message;
          }).join('\n');
          errorMessage = `部署失败，验证错误：\n\n${errorList}`;
        } else if (typeof details === 'string') {
          errorMessage = `部署失败: ${details}`;
        }
      } else if (error.response?.data?.error) {
        errorMessage = `部署失败: ${error.response.data.error}`;
      } else if (error.message) {
        errorMessage = `部署失败: ${error.message}`;
      }

      feedback.error(errorMessage);
    } finally {
      endRuleAction(id);
    }
  };

  const handleUndeploy = async (id: string) => {
    if (!beginRuleAction(id, 'undeploy')) return;
    try {
      if (!await feedback.confirm('确定要取消部署此规则吗？')) return;
      const undeployedRule = await rulesAPI.undeploy(id);
      patchRuleSummary(id, (rule) => ({
        ...rule,
        enabled: false,
        version: undeployedRule.version ?? rule.version,
        status: 'not_deployed',
        updated_at: new Date().toISOString(),
      }));
      await loadRules();
      feedback.success('已取消部署');
    } catch (error) {
      console.error('Failed to undeploy rule:', error);
      await loadRules();
      feedback.error('取消部署失败');
    } finally {
      endRuleAction(id);
    }
  };

  const handleDuplicate = async (id: string) => {
    try {
      const originalRule = await rulesAPI.get(id);
      const newRuleId = `rule_${Date.now()}`;

      const newRule: typeof originalRule = {
        ...originalRule,
        id: newRuleId,
        name: `${originalRule.name || originalRule.id} (副本)`,
        enabled: false,
        status: undefined,
        lifecycle_status: 'draft' as const,
      };

      delete (newRule as any).created_at;
      delete (newRule as any).updated_at;

      const createdRule = await rulesAPI.create(newRule);
      await loadRules();

      if (await feedback.confirm('规则复制成功！是否立即编辑新规则？')) {
        navigate(`/rules/${createdRule.id}/edit`);
      }
    } catch (error) {
      console.error('Failed to duplicate rule:', error);
      feedback.error('复制失败：' + (error instanceof Error ? error.message : '未知错误'));
    }
  };

  const handleEnableDebug = async (id: string) => {
    try {
      const rule = await rulesAPI.get(id);
      await rulesAPI.update(id, { ...rule, debug_mode: true });
      await loadRules();
      feedback.success('已启用调试模式');
    } catch (error) {
      console.error('Failed to enable debug:', error);
      feedback.error('启用调试失败');
    }
  };

  const handleExportSelected = async () => {
    if (selectedRuleIds.size === 0) {
      setMessage({ type: 'error', text: '请先勾选至少一条规则。' });
      return;
    }

    setExporting(true);
    setMessage(null);
    try {
      const selectedIDs = Array.from(selectedRuleIds);
      for (const ruleID of selectedIDs) {
        const rule = await rulesAPI.get(ruleID);
        downloadRuleFile(rule);
      }
      setMessage({ type: 'success', text: `已导出 ${selectedIDs.length} 条规则，每条规则一个 JSON 文件。` });
    } catch (error: any) {
      console.error('Failed to export rules:', error);
      setMessage({ type: 'error', text: `导出失败：${error.message || '未知错误'}` });
    } finally {
      setExporting(false);
    }
  };

  const handleImportClick = () => {
    setMessage(null);
    fileInputRef.current?.click();
  };

  const handleImportFiles = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files || []);
    event.target.value = '';

    if (files.length === 0) {
      return;
    }

    setImporting(true);
    setMessage(null);

    const existingRules = new Map(rules.map((rule) => [rule.id, rule]));
    const reservedIDs = new Set(rules.map((rule) => rule.id));

    let createdCount = 0;
    let updatedCount = 0;
    let skippedCount = 0;

    try {
      for (const file of files) {
        let parsedRule: RuleFlow;
        try {
          const fileContent = await file.text();
          const parsed = JSON.parse(fileContent);
          parsedRule = normalizeImportedRule(parsed, file.name);
        } catch (error: any) {
          skippedCount += 1;
          console.error(`Failed to parse imported rule file ${file.name}:`, error);
          continue;
        }

        const existingRule = existingRules.get(parsedRule.id);
        if (existingRule) {
          const canOverwrite =
            existingRule.status !== 'deployed' &&
            existingRule.status !== 'running' &&
            await feedback.confirm({
              title: `规则 ${parsedRule.id} 已存在`,
              content: '是否覆盖现有规则？点击“取消”将以新 ID 导入。',
            });

          if (canOverwrite) {
            const updatedRule: RuleFlow = {
              ...parsedRule,
              id: existingRule.id,
              status: undefined,
            };
            await rulesAPI.update(existingRule.id, updatedRule);
            updatedCount += 1;
            continue;
          }

          parsedRule = {
            ...parsedRule,
            id: getNextImportedRuleID(parsedRule.id, reservedIDs),
            name: `${parsedRule.name || parsedRule.id} (导入)`,
            status: undefined,
          };
        }

        await rulesAPI.create(parsedRule);
        reservedIDs.add(parsedRule.id);
        createdCount += 1;
      }

      await loadRules();

      const resultParts = [
        createdCount > 0 ? `新增 ${createdCount} 条` : '',
        updatedCount > 0 ? `覆盖 ${updatedCount} 条` : '',
        skippedCount > 0 ? `跳过 ${skippedCount} 个无效文件` : '',
      ].filter(Boolean);

      if (resultParts.length === 0) {
        setMessage({ type: 'error', text: '没有成功导入任何规则，请检查 JSON 文件内容。' });
      } else {
        setMessage({ type: 'success', text: `导入完成：${resultParts.join('，')}。` });
      }
    } catch (error: any) {
      console.error('Failed to import rules:', error);
      setMessage({ type: 'error', text: `导入失败：${error.message || '未知错误'}` });
    } finally {
      setImporting(false);
    }
  };

  const getStatusTone = (status?: string): StatusTone => {
    switch (status) {
      case 'deployed':
      case 'running':
        return 'success';
      case 'error':
        return 'danger';
      default:
        return 'neutral';
    }
  };

  const getStatusText = (status?: string) => {
    switch (status) {
      case 'deployed':
        return '已部署';
      case 'running':
        return '运行中';
      case 'error':
        return '错误';
      default:
        return '未部署';
    }
  };

  const columns: NonNullable<TableProps<RuleSummary>['columns']> = [
      {
        title: (
          <Checkbox
            aria-label="选择当前页全部规则"
            checked={allVisibleSelected}
            onChange={() => toggleSelectAllVisible()}
          />
        ),
        dataIndex: '__select__',
        width: 56,
        render: (_, rule) => (
          <Checkbox
            aria-label={`选择规则 ${rule.name || rule.id}`}
            checked={selectedRuleIds.has(rule.id)}
            onChange={() => toggleRuleSelection(rule.id)}
          />
        ),
      },
      {
        title: '名称',
        dataIndex: 'name',
        ellipsis: true,
        render: (_, rule) => (
          <div>
            <div
              style={{
                fontWeight: 500,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
              title={rule.name || rule.id}
            >
              {rule.name || rule.id}
            </div>
            {rule.description ? (
              <div
                style={{
                  fontSize: '12px',
                  color: '#666',
                  marginTop: '4px',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
                title={rule.description}
              >
                {rule.description}
              </div>
            ) : null}
          </div>
        ),
      },
      {
        title: 'ID',
        dataIndex: 'id',
        ellipsis: true,
        render: (_, rule) => (
          <div
            style={{
              fontSize: '12px',
              color: 'var(--color-text-secondary)',
              fontFamily: 'JetBrains Mono, monospace',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
            title={rule.id}
          >
            {rule.id}
          </div>
        ),
      },
      {
        title: '版本',
        dataIndex: 'version',
        width: 80,
        align: 'center',
        render: (_, rule) => `v${rule.version}`,
      },
      {
        title: '状态',
        dataIndex: 'enabled',
        width: 112,
        align: 'center',
        render: (_, rule) => (
          <CompactButton
            tone={rule.enabled ? 'primary' : 'default'}
            aria-label={`${rule.name || rule.id} 启停状态`}
            disabled={Boolean(pendingRuleActions[rule.id])}
            onClick={() => void (rule.enabled ? handleDisable(rule.id) : handleEnable(rule.id))}
          >
            {rule.enabled ? '已启用' : '已停用'}
          </CompactButton>
        ),
      },
      {
        title: '部署状态',
        dataIndex: 'status',
        width: 110,
        align: 'center',
        render: (_, rule) => (
          <StatusTag tone={getStatusTone(rule.status)}>
            {getStatusText(rule.status)}
          </StatusTag>
        ),
      },
      {
        title: '更新时间',
        dataIndex: 'updated_at',
        width: 168,
        align: 'center',
        render: (_, rule) => (
          <span style={{ fontSize: '12px', color: '#666', whiteSpace: 'nowrap' }}>
            {rule.updated_at ? new Date(rule.updated_at).toLocaleString('zh-CN') : '-'}
          </span>
        ),
      },
      {
        title: '操作',
        dataIndex: '__actions__',
        align: 'center',
        fixed: 'right',
        width: 260,
        render: (_, rule) => {
          const deployed = rule.status === 'deployed' || rule.status === 'running';
          const pendingAction = pendingRuleActions[rule.id];
          const items: ActionMenuItem[] = [
            {
              key: 'lifecycle',
              label: '生命周期管理',
              onClick: () => navigate(`/rules/${rule.id}/lifecycle`),
            },
            {
              key: 'duplicate',
              label: '复制规则',
              onClick: () => void handleDuplicate(rule.id),
            },
            {
              key: 'export',
              label: '导出 JSON',
              onClick: async () => {
                try {
                  downloadRuleFile(await rulesAPI.get(rule.id));
                } catch (error) {
                  console.error('Failed to export single rule:', error);
                  feedback.error('导出失败');
                }
              },
            },
            {
              key: 'debug',
              label: rule.debug_mode ? '调试模式已启用' : '启用调试模式',
              disabled: Boolean(rule.debug_mode),
              onClick: () => void handleEnableDebug(rule.id),
            },
            {
              key: 'delete',
              label: '删除规则',
              danger: true,
              disabled: Boolean(pendingAction),
              onClick: () => void handleDelete(rule.id),
            },
          ];

          const openRuntime = async () => {
            try {
              const instance = await runtimeAPI.getInstanceByRuleID(rule.id);
              if (instance?.id) {
                navigate(`/runtime/${instance.id}/dag`);
              } else {
                feedback.warning('未找到运行实例，请稍后刷新重试');
              }
            } catch (error) {
              console.error('Failed to get instance:', error);
              navigate('/runtime');
            }
          };

          return (
            <div className="rule-row-actions" onClick={(event) => event.stopPropagation()}>
              {deployed ? (
                <CompactButton onClick={() => void openRuntime()}>查看运行态</CompactButton>
              ) : (
                <CompactButton onClick={() => navigate(`/rules/${rule.id}/edit`)}>编辑</CompactButton>
              )}
              <CompactButton
                tone={deployed ? 'default' : 'primary'}
                loading={Boolean(pendingAction)}
                pendingText={pendingAction === 'undeploy' ? '取消中...' : '部署中...'}
                aria-label={pendingAction ? (pendingAction === 'undeploy' ? '取消中...' : '部署中...') : (deployed ? '取消部署' : '部署')}
                onClick={() => void (deployed ? handleUndeploy(rule.id) : handleDeploy(rule.id))}
              >
                {deployed ? '取消部署' : '部署'}
              </CompactButton>
              <ActionMenu label={`${rule.name || rule.id} 更多操作`} items={items} />
            </div>
          );
        },
      },
    ];

  return (
    <PageLayout maxWidth={1500}>
      <input
        ref={fileInputRef}
        type="file"
        accept=".json,application/json"
        multiple
        hidden
        onChange={handleImportFiles}
      />

      <h1 className="sr-only">规则列表</h1>

      {message ? (
        <Alert
          showIcon
          closable
          type={message.type}
          message={message.text}
          onClose={() => setMessage(null)}
          style={{ marginBottom: 12 }}
        />
      ) : null}

      <FilterBar
        summary={`共 ${filteredRules.length} 条 · 已选 ${selectedCount} 条`}
        actions={
          <Space wrap>
            <Button onClick={() => void loadRules()} loading={loading}>刷新</Button>
            <ActionButton loading={importing} pendingText="导入中" onClick={handleImportClick}>导入规则</ActionButton>
            <ActionButton
              loading={exporting}
              pendingText="导出中"
              disabled={selectedCount === 0}
              onClick={() => void handleExportSelected()}
            >
              导出选中{selectedCount > 0 ? `（${selectedCount}）` : ''}
            </ActionButton>
            <Button type="primary" onClick={() => navigate('/rules/new')}>新建规则</Button>
          </Space>
        }
      >
        <Input.Search
          allowClear
          aria-label="搜索规则"
          placeholder="搜索规则名称或 ID"
          value={searchTerm}
          onChange={(event) => setSearchTerm(event.target.value)}
          style={{ flex: '1 1 280px', maxWidth: 520 }}
        />
        <Select
          aria-label="筛选启停状态"
          value={filterEnabled}
          onChange={setFilterEnabled}
          style={{ minWidth: 150 }}
          options={[
            { value: 'all', label: '全部启停状态' },
            { value: 'enabled', label: '已启用' },
            { value: 'disabled', label: '已停用' },
          ]}
        />
        {selectedCount > 0 ? <Button onClick={clearSelection}>清空选择</Button> : null}
      </FilterBar>

      <AsyncState
        loading={loading && rules.length === 0}
        error={loadError || undefined}
        onRetry={() => void loadRules()}
        empty={!loading && !loadError && filteredRules.length === 0}
        emptyDescription={searchTerm || filterEnabled !== 'all' ? '没有匹配的规则，请调整筛选条件。' : '暂无规则，可以先创建第一条规则。'}
        emptyAction={!searchTerm && filterEnabled === 'all' ? <Button type="primary" onClick={() => navigate('/rules/new')}>新建规则</Button> : undefined}
      >
        <div className="console-surface rule-list-table">
          <Table<RuleSummary>
            columns={columns}
            dataSource={paginatedRules}
            loading={loading}
            pagination={false}
            rowKey="id"
            size="middle"
            rowClassName={() => 'rule-list-row'}
            onRow={(record) => ({
              'data-testid': 'rule-row',
              'data-rule-id': record.id,
            } as React.HTMLAttributes<HTMLTableRowElement>)}
            scroll={{ x: 1120 }}
          />
        </div>

        <div className="rule-list-pagination">
          <Space>
            <span>每页条数</span>
            <select
              aria-label="每页条数"
              value={pageSize}
              onChange={(event) => setPageSize(Number(event.target.value))}
              className="rule-list-page-size"
            >
              {[10, 20, 50, 100].map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
            <span>共 {filteredRules.length} 条</span>
          </Space>
          <Space>
            <Button onClick={() => setCurrentPage((prev) => Math.max(1, prev - 1))} disabled={currentPage === 1}>上一页</Button>
            <span className="rule-list-pagination__current">第 {currentPage} / {totalPages} 页</span>
            <Button onClick={() => setCurrentPage((prev) => Math.min(totalPages, prev + 1))} disabled={currentPage === totalPages}>下一页</Button>
          </Space>
        </div>
      </AsyncState>
    </PageLayout>
  );
};

const normalizeImportedRule = (rawRule: any, fileName: string): RuleFlow => {
  if (!rawRule || typeof rawRule !== 'object') {
    throw new Error(`invalid rule json: ${fileName}`);
  }
  if (!rawRule.id || typeof rawRule.id !== 'string') {
    throw new Error(`missing rule id: ${fileName}`);
  }
  if (!Array.isArray(rawRule.nodes)) {
    throw new Error(`invalid nodes: ${fileName}`);
  }

  const normalizedRule: RuleFlow = {
    ...rawRule,
    id: String(rawRule.id).trim(),
    name: typeof rawRule.name === 'string' && rawRule.name.trim() ? rawRule.name : String(rawRule.id).trim(),
    description: typeof rawRule.description === 'string' ? rawRule.description : '',
    version: typeof rawRule.version === 'number' && rawRule.version > 0 ? rawRule.version : 1,
    enabled: Boolean(rawRule.enabled),
    debug_mode: Boolean(rawRule.debug_mode),
    nodes: rawRule.nodes,
    metadata: rawRule.metadata || {},
    source: rawRule.source,
    lifecycle_status: rawRule.lifecycle_status,
    status: undefined,
  };

  delete (normalizedRule as any).created_at;
  delete (normalizedRule as any).updated_at;
  return normalizedRule;
};

const getNextImportedRuleID = (baseID: string, existingIDs: Set<string>) => {
  const normalizedBase = baseID.trim() || `imported_rule_${Date.now()}`;
  if (!existingIDs.has(normalizedBase)) {
    existingIDs.add(normalizedBase);
    return normalizedBase;
  }

  let index = 1;
  let nextID = `${normalizedBase}_import_${index}`;
  while (existingIDs.has(nextID)) {
    index += 1;
    nextID = `${normalizedBase}_import_${index}`;
  }
  existingIDs.add(nextID);
  return nextID;
};

const downloadRuleFile = (rule: RuleFlow) => {
  const jsonContent = JSON.stringify(rule, null, 2);
  const blob = new Blob([jsonContent], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `${sanitizeFileName(rule.id || rule.name || 'rule')}.json`;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
};

const sanitizeFileName = (value: string) => {
  return Array.from(value).map((char) => {
    const code = char.charCodeAt(0);
    const isControlChar = code >= 0 && code <= 31;
    if (isControlChar || '<>:"/\\|?*'.includes(char)) {
      return '_';
    }
    return char;
  }).join('');
};
