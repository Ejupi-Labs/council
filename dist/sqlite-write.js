/** Read write-result metadata exactly without changing SELECT statement modes. */
export function writeStatement(statement) { statement.setReadBigInts(true); return statement; }
//# sourceMappingURL=sqlite-write.js.map