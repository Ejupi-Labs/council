import type { StatementSync } from 'node:sqlite';
/** Read write-result metadata exactly without changing SELECT statement modes. */
export function writeStatement(statement: StatementSync): StatementSync { statement.setReadBigInts(true); return statement; }
