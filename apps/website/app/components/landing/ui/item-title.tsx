import * as React from 'react';
import { cn } from '@/lib/cn';

interface ItemTitleProps extends React.HTMLAttributes<HTMLHeadingElement> {}

const ItemTitle = React.forwardRef<HTMLHeadingElement, ItemTitleProps>(
  ({ className, children, ...props }, ref) => (
    <h3 ref={ref} data-slot="item-title" className={cn('text-sm leading-none font-semibold tracking-tight sm:text-base', className)} {...props}>
      {children}
    </h3>
  )
);
ItemTitle.displayName = 'ItemTitle';

export { ItemTitle };
