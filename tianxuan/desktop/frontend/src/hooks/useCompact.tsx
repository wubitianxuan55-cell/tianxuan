import { createContext, useContext } from "react";

const CompactContext = createContext(false);
export function useCompact() { return useContext(CompactContext); }
export default CompactContext;
