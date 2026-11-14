pipeline {
    agent {
        label 'macos'
    }

    parameters {
        string(name: 'RELEASE_TAG', defaultValue: '', description: 'Release tag (e.g. v1.0.0). Leave blank to auto-generate v1.0.<BUILD_NUMBER>.')
        booleanParam(name: 'PUBLISH_RELEASE', defaultValue: true, description: 'Publish release artifacts to GitHub Releases.')
    }

    environment {
        GITHUB_REPO = 'sahil0017/SYNTRIX'
    }

    options {
        timestamps()
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
                script {
                    def tag = params.RELEASE_TAG?.trim()
                    if (!tag) {
                        tag = env.TAG_NAME ?: "v1.0.${env.BUILD_NUMBER}"
                    }
                    env.VERSION = tag
                }
                echo "Building SYNTRIX ${env.VERSION}"
            }
        }

        stage('Setup Environment') {
            steps {
                sh '''
                    set -euo pipefail
                    clang --version
                    go version
                    swift --version
                    python3 --version
                    python3 -m venv .venv
                    .venv/bin/pip install --upgrade pip
                    .venv/bin/pip install -r requirements.txt
                '''
            }
        }

        stage('Test') {
            steps {
                sh '''
                    set -euo pipefail
                    make -C engine test
                    (cd runtime && go test -v ./...)
                    .venv/bin/python -m unittest discover -s ml -p "test_*.py" -v
                '''
            }
        }

        stage('Build & Bundle App') {
            steps {
                sh '''
                    set -euo pipefail
                    make bundle
                '''
            }
        }

        stage('Package Release') {
            steps {
                sh '''
                    set -euo pipefail
                    chmod +x scripts/package_release.sh
                    ./scripts/package_release.sh "${VERSION}"
                '''
            }
        }

        stage('Publish Release') {
            when {
                expression { return params.PUBLISH_RELEASE }
            }
            steps {
                archiveArtifacts artifacts: 'build/release/*', fingerprint: true
                withCredentials([string(credentialsId: 'github-token', variable: 'GH_TOKEN')]) {
                    sh '''
                        set -euo pipefail
                        if gh release view "${VERSION}" --repo "${GITHUB_REPO}" >/dev/null 2>&1; then
                            gh release upload "${VERSION}" build/release/* --repo "${GITHUB_REPO}" --clobber
                        else
                            gh release create "${VERSION}" build/release/* \
                                --repo "${GITHUB_REPO}" \
                                --title "SYNTRIX ${VERSION}" \
                                --notes "Automated Jenkins macOS release for SYNTRIX ${VERSION}.\n\n- Direct DMG: https://github.com/${GITHUB_REPO}/releases/latest/download/SYNTRIX-macOS.dmg\n- Direct ZIP: https://github.com/${GITHUB_REPO}/releases/latest/download/SYNTRIX-macOS.zip"
                        fi
                    '''
                }
            }
        }
    }

    post {
        success {
            echo "Release ready: https://github.com/${GITHUB_REPO}/releases/tag/${env.VERSION}"
            echo "Direct download: https://github.com/${GITHUB_REPO}/releases/latest/download/SYNTRIX-macOS.dmg"
        }
        always {
            sh 'rm -rf .venv'
        }
    }
}
