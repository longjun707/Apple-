import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/cn'

const variants = {
  primary: 'bg-apple-blue text-white hover:bg-blue-600 active:bg-blue-700 shadow-sm shadow-apple-blue/20',
  secondary: 'bg-gray-100 text-gray-700 hover:bg-gray-200 active:bg-gray-300',
  outline: 'border border-gray-200 text-gray-700 bg-white hover:bg-gray-50 active:bg-gray-100 shadow-sm',
  danger: 'bg-red-500 text-white hover:bg-red-600 active:bg-red-700 shadow-sm shadow-red-500/20',
  ghost: 'text-gray-500 hover:bg-gray-100 hover:text-gray-700 active:bg-gray-200',
} as const

const sizes = {
  sm: 'px-3 py-1.5 text-[13px] rounded-lg gap-1.5',
  md: 'px-4 py-2 text-sm rounded-lg gap-2',
  lg: 'px-5 py-2.5 text-[15px] rounded-xl gap-2',
} as const

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants
  size?: keyof typeof sizes
  loading?: boolean
  icon?: ReactNode
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'primary', size = 'md', loading, icon, children, className, disabled, ...props }, ref) => {
    return (
      <button
        ref={ref}
        disabled={disabled || loading}
        className={cn(
          'inline-flex items-center justify-center font-medium transition-all duration-150 select-none',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apple-blue/30 focus-visible:ring-offset-1',
          'disabled:opacity-50 disabled:cursor-not-allowed disabled:shadow-none',
          variants[variant],
          sizes[size],
          className,
        )}
        {...props}
      >
        {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : icon}
        {children}
      </button>
    )
  },
)

Button.displayName = 'Button'
export default Button
