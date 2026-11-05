/** Require unambiguous object keys after the caller has parsed valid JSON. */
export function assertUniqueObjectKeys(json: string, label: string): void {
  const scopes: Set<string>[] = [];
  for (let index = 0; index < json.length; index++) {
    if (json[index] === '{') scopes.push(new Set());
    else if (json[index] === '}') scopes.pop();
    else if (json[index] === '"') {
      const start = index;
      while (++index < json.length) {
        if (json[index] === '\\') index++;
        else if (json[index] === '"') break;
      }
      let next = index + 1;
      while (/\s/.test(json[next] ?? '')) next++;
      if (json[next] === ':') {
        const key = JSON.parse(json.slice(start, index + 1)) as string;
        const scope = scopes.at(-1)!;
        if (scope.has(key)) throw new Error(`${label} contains duplicate JSON object keys.`);
        scope.add(key);
      }
    }
  }
}
