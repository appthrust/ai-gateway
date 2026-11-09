import { useText, type Translate } from '../../i18n'
import type { BlockReason, PickerOption } from '../../model/model-picker'
import { PROVIDER_STATE_MESSAGE } from '../../model/providers'
import { Field, Notice, Select } from '../../ui'

/** Why a model cannot be picked, in product words. */
export function blockReasonText(t: Translate, reason: BlockReason): string {
  if (reason.kind === 'policy') {
    return reason.patterns.length > 0
      ? t('aiGateway.picker.blocked.policyRule', { provider: reason.provider, patterns: reason.patterns.join(', ') })
      : t('aiGateway.picker.blocked.policy')
  }
  return t('aiGateway.picker.blocked.provider', { provider: reason.provider, state: t(PROVIDER_STATE_MESSAGE[reason.state]) })
}

/** One picker row: provider / model, the default marker, and the block reason under a greyed row. */
export function ModelOptionLabel({ option }: { option: PickerOption }) {
  const { t } = useText()
  return (
    <span className="flex flex-col">
      <span>
        <span className="font-mono text-xs">{option.provider ? `${option.provider} / ${option.model}` : option.model}</span>
        {option.isDefault ? <span className="ml-2 text-xs text-accent-strong">{t('aiGateway.models.default')}</span> : null}
      </span>
      {option.blocked ? <span className="text-xs">{blockReasonText(t, option.blocked)}</span> : null}
    </span>
  )
}

export function ModelPicker({
  id,
  options,
  value,
  onChange,
  loading,
  listFailed,
}: {
  id: string
  options: readonly PickerOption[]
  value: string | undefined
  onChange: (value: string) => void
  loading: boolean
  listFailed: boolean
}) {
  const { t } = useText()
  const blocked = options.filter((option) => option.blocked).length
  return (
    <div className="space-y-2">
      <Field
        id={id}
        label={t('aiGateway.field.model')}
        hint={loading ? t('aiGateway.picker.loading') : blocked > 0 ? t('aiGateway.picker.blockedCount', { count: blocked }) : t('aiGateway.picker.hint')}
      >
        <Select
          id={id}
          value={value}
          onChange={onChange}
          placeholder={t(options.length === 0 ? 'aiGateway.picker.empty' : 'aiGateway.picker.placeholder')}
          disabled={options.length === 0}
          options={options.map((option) => ({ value: option.value, disabled: option.blocked !== null, label: <ModelOptionLabel option={option} /> }))}
        />
      </Field>
      {listFailed ? <Notice tone="warning">{t('aiGateway.picker.listFailed')}</Notice> : null}
    </div>
  )
}
