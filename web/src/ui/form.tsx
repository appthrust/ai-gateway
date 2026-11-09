import { Check, ChevronDown } from 'lucide-react'
import { Select as RSelect } from 'radix-ui'
import type { InputHTMLAttributes, ReactNode, Ref, TextareaHTMLAttributes } from 'react'
import { useText } from '../i18n'
import { cn } from './cn'

/** Control look without width; inputs add `w-full`. */
export const controlClasses =
  'block rounded-md border border-stone-300 bg-white px-3 text-sm text-stone-900 shadow-xs transition-colors placeholder:text-stone-400 hover:border-stone-400 focus:border-accent focus:outline-none focus-visible:outline-2 focus-visible:outline-offset-0 focus-visible:outline-accent disabled:cursor-not-allowed disabled:bg-stone-100 disabled:text-stone-500 aria-invalid:border-bad aria-invalid:focus-visible:outline-bad'

/** `${id}-error` when invalid, else `${id}-hint`; Field renders those ids. */
const describedBy = (id: string | undefined, invalid: boolean | undefined) => (id ? `${id}-${invalid ? 'error' : 'hint'}` : undefined)

/**
 * Labelled control. The control itself gets `id`; hint and error are rendered
 * with ids `${id}-hint` / `${id}-error`, which TextInput / Select wire through
 * aria-describedby from their own `id`.
 */
export function Field({
  id,
  label,
  hint,
  error,
  required,
  className,
  children,
}: {
  id: string
  label: ReactNode
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  className?: string
  children: ReactNode
}) {
  const { t } = useText()
  return (
    <div className={className}>
      <label htmlFor={id} className="flex items-center gap-2 text-sm font-medium text-stone-800">
        {label}
        {required ? <span className="rounded bg-stone-100 px-1.5 py-px text-[11px] font-medium text-stone-500">{t('aiGateway.common.required')}</span> : null}
      </label>
      <div className="mt-1.5">{children}</div>
      {error ? (
        <p id={`${id}-error`} className="mt-1.5 text-xs text-bad-ink">
          {error}
        </p>
      ) : hint ? (
        <p id={`${id}-hint`} className="mt-1.5 text-xs text-stone-500">
          {hint}
        </p>
      ) : null}
    </div>
  )
}

export function TextInput({
  invalid,
  className,
  ref,
  ...rest
}: InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean; ref?: Ref<HTMLInputElement> }) {
  return (
    <input
      ref={ref}
      aria-invalid={invalid || undefined}
      aria-describedby={describedBy(rest.id, invalid)}
      className={cn(controlClasses, 'h-10 w-full md:h-9', className)}
      {...rest}
    />
  )
}

/** Multi-line text; wires `${id}-hint` / `${id}-error` like TextInput. */
export function TextArea({ invalid, className, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }) {
  return (
    <textarea
      aria-invalid={invalid || undefined}
      aria-describedby={describedBy(rest.id, invalid)}
      className={cn(controlClasses, 'min-h-24 w-full resize-y py-2 leading-relaxed', className)}
      {...rest}
    />
  )
}

/** `indent`: nesting level, applied to the menu row only (the closed trigger shows the plain label). */
export type SelectOption = { value: string; label: ReactNode; disabled?: boolean; indent?: number }

/** Radix Select. `value` undefined shows the placeholder; option values must be non-empty. */
export function Select({
  id,
  value,
  onChange,
  options,
  placeholder,
  invalid,
  disabled,
  className,
  'aria-label': ariaLabel,
}: {
  id?: string
  value: string | undefined
  onChange: (value: string) => void
  options: SelectOption[]
  placeholder?: string
  invalid?: boolean
  disabled?: boolean
  className?: string
  'aria-label'?: string
}) {
  return (
    <RSelect.Root value={value ?? ''} onValueChange={onChange} disabled={disabled}>
      <RSelect.Trigger
        id={id}
        aria-label={ariaLabel}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy(id, invalid)}
        className={cn(controlClasses, 'flex h-10 w-full items-center justify-between gap-2 text-left data-placeholder:text-stone-400 md:h-9', className)}
      >
        <span className="min-w-0 truncate">
          <RSelect.Value placeholder={placeholder} />
        </span>
        <RSelect.Icon asChild>
          <ChevronDown aria-hidden className="size-4 shrink-0 text-stone-500" />
        </RSelect.Icon>
      </RSelect.Trigger>
      <RSelect.Portal>
        <RSelect.Content
          position="popper"
          sideOffset={4}
          className="z-50 max-h-(--radix-select-content-available-height) min-w-(--radix-select-trigger-width) overflow-hidden rounded-md border border-line bg-white shadow-lg animate-pop-in"
        >
          <RSelect.Viewport className="p-1">
            {options.map((option) => (
              <RSelect.Item
                key={option.value}
                value={option.value}
                disabled={option.disabled}
                style={option.indent ? { paddingLeft: `${2 + option.indent * 0.875}rem` } : undefined}
                className="relative flex min-h-10 cursor-default items-center rounded py-1.5 pr-3 pl-8 text-sm text-stone-800 outline-none select-none md:min-h-0 data-disabled:text-stone-400 data-highlighted:bg-accent-soft data-highlighted:text-accent-strong"
              >
                <RSelect.ItemIndicator className="absolute left-2 inline-flex">
                  <Check aria-hidden className="size-4" />
                </RSelect.ItemIndicator>
                <RSelect.ItemText>{option.label}</RSelect.ItemText>
              </RSelect.Item>
            ))}
          </RSelect.Viewport>
        </RSelect.Content>
      </RSelect.Portal>
    </RSelect.Root>
  )
}
