import { useId, type ButtonHTMLAttributes, type InputHTMLAttributes, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'

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
