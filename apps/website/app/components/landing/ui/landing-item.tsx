import * as React from 'react';
import { cn } from '@/lib/cn';

interface LandingItemProps extends React.HTMLAttributes<HTMLDivElement> {}

const LandingItem = React.forwardRef<HTMLDivElement, LandingItemProps>(
  ({ className, children, ...props }, ref) => (
    <div ref={ref} data-slot="item" className={cn('text-foreground flex flex-col gap-4 p-4', className)} {...props}>
      {children}
    </div>
  )
);
LandingItem.displayName = 'LandingItem';

export { LandingItem };
