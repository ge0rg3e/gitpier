import * as React from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/cn';

interface AccordionContextValue {
  openItems: Set<string>;
  toggle: (value: string) => void;
}

const AccordionContext = React.createContext<AccordionContextValue>({
  openItems: new Set(),
  toggle: () => {},
});

function Accordion({
  type = 'single',
  collapsible = false,
  className,
  children,
  ...props
}: {
  type?: 'single' | 'multiple';
  collapsible?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  const [openItems, setOpenItems] = React.useState<Set<string>>(new Set());

  const toggle = React.useCallback(
    (value: string) => {
      setOpenItems((prev) => {
        const next = new Set(prev);
        if (next.has(value)) {
          if (collapsible) next.delete(value);
        } else {
          if (type === 'single') next.clear();
          next.add(value);
        }
        return next;
      });
    },
    [type, collapsible]
  );

  return (
    <AccordionContext.Provider value={{ openItems, toggle }}>
      <div className={className} {...props}>
        {children}
      </div>
    </AccordionContext.Provider>
  );
}

function AccordionItem({
  value,
  className,
  children,
  ...props
}: {
  value: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn('not-last:border-b', className)} data-state={undefined} {...props}>
      {children}
    </div>
  );
}

function AccordionTrigger({
  className,
  children,
  ...props
}: {
  className?: string;
  children: React.ReactNode;
}) {
  const { openItems, toggle } = React.useContext(AccordionContext);
  const item = React.useContext(AccordionItemContext);
  const isOpen = openItems.has(item.value);

  return (
    <button
      type="button"
      onClick={() => toggle(item.value)}
      className={cn(
        'w-full flex items-center justify-between py-2.5 text-left text-sm font-medium transition-all outline-none',
        className
      )}
      data-state={isOpen ? 'open' : 'closed'}
      {...props}
    >
      {children}
      <ChevronDown
        className={cn(
          'size-4 shrink-0 transition-transform duration-200',
          isOpen && 'rotate-180'
        )}
      />
    </button>
  );
}

const AccordionItemContext = React.createContext<{ value: string }>({ value: '' });

function AccordionItemWrapper({
  value,
  className,
  children,
  ...props
}: {
  value: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <AccordionItemContext.Provider value={{ value }}>
      <div className={cn('not-last:border-b', className)} {...props}>
        {children}
      </div>
    </AccordionItemContext.Provider>
  );
}

function AccordionContent({
  className,
  children,
  ...props
}: {
  className?: string;
  children: React.ReactNode;
}) {
  const { openItems } = React.useContext(AccordionContext);
  const item = React.useContext(AccordionItemContext);
  const isOpen = openItems.has(item.value);

  if (!isOpen) return null;

  return (
    <div
      className={cn('text-sm overflow-hidden animate-accordion-down', className)}
      data-state="open"
      {...props}
    >
      <div className="pt-0 pb-2.5 [&_a]:hover:text-foreground [&_a]:underline [&_a]:underline-offset-3 [&_p:not(:last-child)]:mb-4">
        {children}
      </div>
    </div>
  );
}

export { Accordion, AccordionItemWrapper as AccordionItem, AccordionTrigger, AccordionContent };
