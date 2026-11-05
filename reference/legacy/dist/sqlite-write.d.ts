import type { StatementSync } from 'node:sqlite';
/** Read write-result metadata exactly without changing SELECT statement modes. */
export declare function writeStatement(statement: StatementSync): StatementSync;
