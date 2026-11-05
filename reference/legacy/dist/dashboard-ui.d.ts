/** Render complete valid ballots as readable text while preserving other messages verbatim. */
export declare function readableRoomMessage(body: string): string;
/** Self-contained browser client: no external resources and no HTML insertion of room data. */
export declare function dashboardHtml(nonce: string): string;
