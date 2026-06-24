import { listCustomProviderNames } from '../../shared/configUtils';
import { OcrConfig, ProviderEntry } from '../../shared/types';

interface Props {
  config: OcrConfig | null;
  onAdd: () => void;
  onEdit: (name: string) => void;
  onActivate: (name: string) => void;
  onDelete: (name: string) => void;
}

function formatModels(entry: ProviderEntry): string {
  const models = entry.models ?? [];
  if (models.length > 0) return models.join(', ');
  return entry.model ?? '—';
}

export function CustomProviderManager({ config, onAdd, onEdit, onActivate, onDelete }: Props) {
  const names = listCustomProviderNames(config);
  const activeProvider = config?.provider ?? '';

  return (
    <div class="custom-provider-manager">
      <div class="custom-provider-manager-header">
        <div>
          <h2 class="custom-provider-manager-title">自定义 Provider</h2>
          <p class="custom-provider-manager-desc">管理自建 LLM 网关与兼容端点，可切换为当前审查模型。</p>
        </div>
        <button type="button" class="btn-primary" onClick={onAdd}>添加</button>
      </div>

      {names.length === 0 ? (
        <div class="custom-provider-empty">
          <p>暂无自定义 Provider</p>
          <button type="button" class="btn-default" onClick={onAdd}>添加自定义 Provider</button>
        </div>
      ) : (
        <div class="custom-provider-list">
          {names.map((name) => {
            const entry = config?.customProviders[name];
            if (!entry) return null;
            const isActive = activeProvider === name;
            return (
              <div key={name} class={`custom-provider-card${isActive ? ' active' : ''}`}>
                <div class="custom-provider-card-main">
                  <div class="custom-provider-card-title">
                    <span class="custom-provider-name">{name}</span>
                    {isActive && <span class="custom-provider-badge">当前使用</span>}
                  </div>
                  <div class="custom-provider-card-meta">
                    <span>{entry.protocol || '—'}</span>
                    <span class="custom-provider-card-dot">·</span>
                    <span class="custom-provider-card-url" title={entry.url}>{entry.url || '—'}</span>
                  </div>
                  <div class="custom-provider-card-model">模型：{formatModels(entry)}</div>
                </div>
                <div class="custom-provider-card-actions">
                  <button type="button" class="btn-text" onClick={() => onEdit(name)}>编辑</button>
                  {!isActive && (
                    <button type="button" class="btn-text" onClick={() => onActivate(name)}>设为当前</button>
                  )}
                  <button type="button" class="btn-text danger" onClick={() => onDelete(name)}>删除</button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
