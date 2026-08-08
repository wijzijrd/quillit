export { default as FormField } from "./FormField.vue"

export interface ValidationResult {
  message: string
  type: "error" | "success"
}

export type Validate = (
  value: string | number
) => ValidationResult | null | undefined | Promise<ValidationResult | null | undefined>
