import * as React from 'react';
import { cn } from '@/lib/cn';

interface ItemDescriptionProps extends React.HTMLAttributes<HTMLDivElement> {}

const ItemDescription = React.forwardRef<HTMLDivElement, ItemDescriptionProps>(
  ({ className, children, ...props }, ref) => (
    <div ref={ref} data-slot="item-description" className={cn('text-muted-foreground flex max-w-[240px] flex-col gap-2 text-sm text-balance', className)} {...props}>
      {children}
    </div>
  )
);
ItemDescription.displayName = 'ItemDescription';

export { ItemDescription };
