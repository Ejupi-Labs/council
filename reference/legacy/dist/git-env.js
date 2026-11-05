const repositoryVariables = new Set([
    'GIT_ALTERNATE_OBJECT_DIRECTORIES', 'GIT_OBJECT_DIRECTORY', 'GIT_DIR', 'GIT_WORK_TREE',
    'GIT_IMPLICIT_WORK_TREE', 'GIT_GRAFT_FILE', 'GIT_INDEX_FILE', 'GIT_NO_REPLACE_OBJECTS',
    'GIT_REPLACE_REF_BASE', 'GIT_PREFIX', 'GIT_INTERNAL_SUPER_PREFIX', 'GIT_SHALLOW_FILE',
    'GIT_COMMON_DIR', 'GIT_NAMESPACE', 'GIT_ATTR_SOURCE',
    'GIT_LITERAL_PATHSPECS', 'GIT_GLOB_PATHSPECS', 'GIT_NOGLOB_PATHSPECS', 'GIT_ICASE_PATHSPECS',
]);
/** Keep child cwd authoritative instead of inheriting a caller's Git checkout/index. */
export function sanitizedGitEnvironment(environment = process.env) {
    return Object.fromEntries(Object.entries(environment).filter(([key]) => {
        const name = key.toUpperCase();
        return !repositoryVariables.has(name) && name !== 'GIT_CONFIG' && !name.startsWith('GIT_CONFIG_');
    }));
}
//# sourceMappingURL=git-env.js.map