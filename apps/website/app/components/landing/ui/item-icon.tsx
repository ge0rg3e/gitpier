import * as React from 'react';
import { cn } from '@/lib/cn';

interface ItemIconProps extends React.HTMLAttributes<HTMLDivElement> {}

const ItemIcon = React.forwardRef<HTMLDivElement, ItemIconProps>(
  ({ className, children, ...props }, ref) => (
    <div ref={ref} data-slot="item-icon" className={cn('flex items-center self-start', className)} {...props}>
      {children}
    </div>
  )
);
ItemIcon.displayName = 'ItemIcon';

export { ItemIcon };
