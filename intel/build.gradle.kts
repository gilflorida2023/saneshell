plugins {
    id("java")
    id("application")
    id("com.github.gmazzo.buildconfig") version "5.5.1"
}

group = "com.saneshell"
version = providers.gradleProperty("version").getOrElse("0.1.0")
val protocolVersion = providers.gradleProperty("protocolVersion").getOrElse("1").toInt()

java {
    toolchain.languageVersion.set(JavaLanguageVersion.of(21))
}

application {
    mainClass.set("com.saneshell.intel.Main")
}

repositories {
    mavenCentral()
}

dependencies {
    implementation("com.fasterxml.jackson.core:jackson-databind:2.17.0")
    implementation("org.slf4j:slf4j-api:2.0.13")
    implementation("org.slf4j:slf4j-simple:2.0.13")
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.2")
}

tasks.withType<Test> {
    useJUnitPlatform()
}

buildConfig {
    buildConfigField("String", "VERSION", "\"${project.version}\"")
    buildConfigField("int", "PROTOCOL_VERSION", protocolVersion.toString())
}

val fatJar = task("fatJar", type = Jar::class) {
    dependsOn("compileJava")
    from(sourceSets.main.get().output)
    duplicatesStrategy = DuplicatesStrategy.WARN
    manifest {
        attributes("Main-Class" to application.mainClass.get())
    }
}

tasks.named("build") {
    dependsOn(fatJar)
}
