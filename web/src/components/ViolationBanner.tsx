import type { TraceVerdict } from "../types/trace";
import styles from "./ViolationBanner.module.css";

export function ViolationBanner({ verdict }: { verdict: TraceVerdict }) {
  if (verdict.passed || !verdict.violation) return null;
  return (
    <div className={styles.banner}>
      {verdict.violation.checker}: {verdict.violation.detail}
    </div>
  );
}
