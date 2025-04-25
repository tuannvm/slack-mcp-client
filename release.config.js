module.exports = {
    branches: ['main'],
    plugins: [
        [
            '@semantic-release/commit-analyzer',
            {
                preset: 'angular',
                releaseRules: [
                    // Keep conventional commit standard rules first
                    { type: 'feat', release: 'minor' },
                    { type: 'fix', release: 'patch' },
                    { type: 'perf', release: 'patch' },
                    { type: 'docs', scope: 'README', release: 'patch' },
                    { type: 'refactor', release: 'patch' },
                    { type: 'chore', scope: 'deps', release: 'patch' },
                    // Consider any changes to Go files as a patch release (fallback)
                    { files: ['**/*.go'], release: 'patch' },
                    { type: 'release', release: 'patch' }
                ]
            }
        ],
        '@semantic-release/release-notes-generator',
        '@semantic-release/changelog',
        '@semantic-release/github',
        [
            '@semantic-release/git',
            {
                assets: ['CHANGELOG.md', 'package.json'],
                message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}',
            },
        ],
    ],
    preset: 'angular',
}; 
