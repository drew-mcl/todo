import { createContext, useContext, useMemo } from "react";
import { assignHues, topicHue } from "./topic";

/**
 * The assignment depends on the whole set of topics, so it is worked out once
 * from the sidebar data and read wherever a dot is drawn.
 */
const HueContext = createContext<Map<string, number> | null>(null);

export function HueProvider({
  topics,
  children,
}: {
  topics: string[];
  children: React.ReactNode;
}) {
  const key = topics.join(" ");
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const map = useMemo(() => assignHues(topics), [key]);
  return <HueContext.Provider value={map}>{children}</HueContext.Provider>;
}

export function useHue(topic: string): number {
  const map = useContext(HueContext);
  return map?.get(topic) ?? topicHue(topic);
}
