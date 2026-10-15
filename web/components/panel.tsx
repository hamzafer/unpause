import { cn } from "cn";
import { Card } from "@/components/ui/card";

/** A quiet card: hairline border, no shadow, title in sentence case. */
export function Panel({
  title,
  action,
  className,
  children,
  id,
}: {
  title: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
  id?: string;
}) {
  const headingId = id ? `${id}-title` : undefined;
  return (
    <Card
      className={cn("min-w-0 gap-0 rounded-lg py-0 shadow-none", className)}
      aria-labelledby={headingId}
      role="region"
    >
      <div className="flex min-h-12 items-center justify-between gap-3 px-4 pt-3 pb-2">
        <h2 id={headingId} className="text-sm font-medium text-foreground">
          {title}
        </h2>
        {action}
      </div>
      {children}
    </Card>
  );
}
