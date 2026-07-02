import { listCustomProviderNames } from '../../shared/configUtils';
import { OcrConfig, ProviderEntry } from '../../shared/types';
import { useT } from '../I18nProvider';

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
  const t = useT();
  const names = listCustomProviderNames(config);
  const activeProvider = config?.provider ?? '';

  return (
    <div class="custom-provider-manager">
      <div class="custom-provider-manager-header">
        <div>
          <h2 class="custom-provider-manager-title">{t('cmp.custom.title')}</h2>
          <p class="custom-provider-manager-desc">{t('cmp.custom.desc')}</p>
        </div>
        <button type="button" class="btn-primary" onClick={onAdd}>{t('cmp.custom.add')}</button>
      </div>

      {names.length === 0 ? (
        <div class="custom-provider-empty">
          <p>{t('cmp.custom.empty')}</p>
          <button type="button" class="btn-default" onClick={onAdd}>{t('cmp.custom.addFirst')}</button>
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
                    {isActive && <span class="custom-provider-badge">{t('cmp.custom.currentUse')}</span>}
                  </div>
                  <div class="custom-provider-card-meta">
                    <span>{entry.protocol || '—'}</span>
                    <span class="custom-provider-card-dot">·</span>
                    <span class="custom-provider-card-url" title={entry.url}>{entry.url || '—'}</span>
                  </div>
                  <div class="custom-provider-card-model">{t('cmp.custom.model')}: {formatModels(entry)}</div>
                </div>
                <div class="custom-provider-card-actions">
                  <button type="button" class="btn-text" onClick={() => onEdit(name)}>{t('cmp.custom.edit')}</button>
                  {!isActive && (
                    <button type="button" class="btn-text" onClick={() => onActivate(name)}>{t('cmp.custom.setCurrent')}</button>
                  )}
                  <button type="button" class="btn-text danger" onClick={() => onDelete(name)}>{t('cmp.custom.delete')}</button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
