import { useId, type CSSProperties, type ButtonHTMLAttributes, type InputHTMLAttributes, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'

export function Button({ variant = 'outlined', className = '', type = 'button', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'filled' | 'tonal' | 'outlined' | 'text' }) {
  return <button type={type} className={`md-button md-button--${variant} ${className}`} {...props} />
}

type FieldProps = { label: string; supportingText?: string }
export function TextField({ label, supportingText, id, ...props }: InputHTMLAttributes<HTMLInputElement> & FieldProps) {
  const generatedId = useId()
  const fieldId = id ?? generatedId
  return <div className="md-field"><label htmlFor={fieldId}>{label}</label><input id={fieldId} aria-describedby={supportingText ? `${fieldId}-support` : undefined} {...props} />{supportingText && <small id={`${fieldId}-support`}>{supportingText}</small>}</div>
}
export function TextArea({ label, supportingText, id, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement> & FieldProps) {
  const generatedId = useId()
  const fieldId = id ?? generatedId
  return <div className="md-field"><label htmlFor={fieldId}>{label}</label><textarea id={fieldId} aria-describedby={supportingText ? `${fieldId}-support` : undefined} {...props} />{supportingText && <small id={`${fieldId}-support`}>{supportingText}</small>}</div>
}

export function SelectField({ label, supportingText, id, ...props }: SelectHTMLAttributes<HTMLSelectElement> & FieldProps) {
  const generatedId = useId()
  const fieldId = id ?? generatedId
  return <div className="md-field"><label htmlFor={fieldId}>{label}</label><select id={fieldId} aria-describedby={supportingText ? `${fieldId}-support` : undefined} {...props} />{supportingText && <small id={`${fieldId}-support`}>{supportingText}</small>}</div>
}

export function LinearProgress({ id, value, max, label }: { id?: string; value: number; max: number; label: string }) {
  const total = Math.max(1, max)
  const completed = Math.min(total, Math.max(0, value))
  const valueText = `Обработано ${completed.toLocaleString('ru-RU')} из ${max.toLocaleString('ru-RU')}`
  return <div id={id} className="md-linear-progress">
    <div className="md-linear-progress__caption" aria-hidden="true"><span>Обработано</span><span>{completed.toLocaleString('ru-RU')} / {max.toLocaleString('ru-RU')}</span></div>
    <div className="md-linear-progress__bar" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={total} aria-valuenow={completed} aria-valuetext={valueText} style={{ '--progress': `${completed / total * 100}%` } as CSSProperties}>
      <span className="md-linear-progress__active" />
      {completed < total && <span className="md-linear-progress__track" />}
    </div>
  </div>
}
