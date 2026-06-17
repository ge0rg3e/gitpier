import * as React from 'react';
import { cn } from '@/lib/cn';

type GlowVariant = 'top' | 'above' | 'bottom' | 'below' | 'center';

interface GlowProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: GlowVariant;
}

const variantClass: Record<GlowVariant, string> = {
  top: 'top-0',
  above: '-top-[128px]',
  bottom: 'bottom-0',
  below: '-bottom-[128px]',
  center: 'top-[50%]',
};

const Glow = React.forwardRef<HTMLDivElement, GlowProps>(
  ({ className, variant = 'top', ...props }, ref) => (
    <div ref={ref} data-slot="glow" className={cn('absolute w-full', variantClass[variant], className)} {...props}>
      <div
        className={cn(
          'from-brand/50 to-brand/0 absolute left-1/2 h-[256px] w-[60%] -translate-x-1/2 scale-[2.5] rounded-[50%] bg-radial from-10% to-60% opacity-20 sm:h-[512px] dark:opacity-100',
          variant === 'center' && '-translate-y-1/2'
        )}
      />
      <div
        className={cn(
          'from-brand/30 to-brand/0 absolute left-1/2 h-[128px] w-[40%] -translate-x-1/2 scale-[2] rounded-[50%] bg-radial from-10% to-60% opacity-20 sm:h-[256px] dark:opacity-100',
          variant === 'center' && '-translate-y-1/2'
        )}
      />
    </div>
  )
);
Glow.displayName = 'Glow';

export { Glow };
