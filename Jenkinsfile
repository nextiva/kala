node('slave_golang') {

    stage('Checkout') {
        checkout scm
    }

    stage('Init') {
        dir('configs') {
            script {
                env.GOLANG_IMAGE_NAME = readFile 'GOLANG_IMAGE_NAME'
                env.GOLANG_IMAGE_NAME = env.GOLANG_IMAGE_NAME.trim()

                env.GOLANG_IMAGE_VERSION = readFile 'GOLANG_IMAGE_VERSION'
                env.GOLANG_IMAGE_VERSION = env.GOLANG_IMAGE_VERSION.trim()

                env.DOCKER_IMAGE_NAME = readFile 'DOCKER_IMAGE_NAME'
                env.DOCKER_IMAGE_NAME = env.DOCKER_IMAGE_NAME.trim()

                env.NO_DEPS_IMAGE_NAME = env.DOCKER_IMAGE_NAME + '-no-deps'
                env.WITH_DEPS_IMAGE_NAME = env.DOCKER_IMAGE_NAME + '-with-deps'

                env.VERSION = readFile 'VERSION'
                env.VERSION = env.VERSION.trim()

                env.ANSIBLE_REMOTE_USER = "jenkins"
            }
        }

        dir('deployments/inventories/dev') {
            script {
                // For now, there's only one host
                def file = readFile 'hosts'
                def lines = file.readLines()
                env.TARGET_HOST = lines.get(1).trim()
            }
        }
    }

    stage('Build Image Without Dependencies') {
        try {
            sh 'make no-deps-image'
        } catch (err) {
            echo "An error occurred while building the no-deps image: ${err}"
            sh 'make clean'
            throw err
        }
    }

    stage('Download Dependencies Into Image') {
        try {
            sh 'make run-download-deps'
        } catch (err) {
            echo "An error occurred while downloading dependencies into the image: ${err}"
            sh 'make clean'
            throw err
        }
    }

    stage('Lint') {
        try {
            sh 'make lint-image'
        } catch (err) {
            echo "An error occurred while linting the image: ${err}"
            sh 'make clean'
            throw err
        }
    }

    stage('Test') {
        try {
            sh 'make test-image'
        } catch (err) {
            echo "An error occurred while testing the image: ${err}"
            sh 'make clean'
            throw err
        }
    }

    stage('Vet') {
        try {
            sh 'make vet-image'
        } catch (err) {
            echo "An error occurred while vetting the image: ${err}"
            sh 'make clean'
            throw err
        }
    }

    stage('Build Server Image') {
        try {
            sh 'make server-image'
        } catch (err) {
            echo "An error occurred while building the server image: ${err}"
            sh 'make clean'
            throw err
        }
    }

    stage('Push Docker Image') {
        try {
            docker.withRegistry("https://docker.nextiva.xyz", 'nextivaRegistry') {
                docker.image("${env.DOCKER_IMAGE_NAME}:${env.VERSION}").push()
            }
        } catch (err) {
            echo "An error occurred while pushing the docker image: ${err}"
            throw err
        } finally {
            sh 'make clean'
        }
    }

    stage('Deploy Docker Image') {
        dir('deployments') {

            sh "ssh-keyscan -H ${env.TARGET_HOST} >> ~/.ssh/known_hosts"
            sshagent(['jenkins-master-server-ssh-key']) {
                configFileProvider([configFile(fileId: 'ansible-password-datastuff', variable: 'ANSIBLE_PASSWORD')]) {
                    sh "ansible-playbook -i inventories/dev/hosts --vault-password-file $ANSIBLE_PASSWORD --tags deploy rosterorchestrationservice.yml"
                }
            }

        }
    }
}