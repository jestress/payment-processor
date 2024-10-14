# Decoupling of the validation logic from the main program

The validation logic that was previously handled within the handleRequest method has been moved to a separate validator to decouple the validation process and make the system more modular and testable.

# Proceeding with manual Dependency Injection instead of using a DIC

Although dependency injection (DI) offers flexibility with modularity, using a Dependency Injection Container (DIC) could introduce unnecessary complexity for this solution. Therefore, dependencies will be injected manually via constructors.

# Creating a compact 'tcpServer' for moving the TCP logic from the main program

To further decouple TCP logic from the main program, a stand-alone TcpServer has been created. This improves modularity and makes the overall solution more testable.

# Using channels for asynchronous inter-routine communication

Asynchronous communication between Go routines within the solution is handled using channels. For a compact-scale solution such as this, channels should enhance the readability and maintainability of the code. Although a Context could also have been used for managing concurrency, since the server is expected to process only one request per connection, using channels simplifies the design, adhering to the YAGNI (You Aren’t Gonna Need It) principle.

# Moving the test initialization logic from TestsMain(m *testing.M) to setupServer(t)

TestsMain runs once and executes all tests. In scenarios where the TcpServer is frequently shut down and restarted, moving the initialization logic to setupServer(t) ensures that no unit test shuts down the TcpServer in a way that prevents other tests from using it.

# Unit test structure

Each module contains its corresponding unit tests, which reside in the same directory as the module itself.

# Unit tests for the TcpServer

Due to the nature of the solution, comprehensive unit tests have not been written for the TcpServer component. Instead, server orchestration is tested via integration tests (in main_test.go). Repeating these scenarios in unit tests for the TcpServer would be redundant and counterproductive.

# Integration test structure

The main_test.go file contains the integration test scenarios for the solution. Scenarios such as "Invalid Request" and "Invalid Amount" are defined as integration tests in main_test.go and as unit tests in the respective module's test file. This ensures that each module is robust on its own while also verifying its integration with other components.

# Singularized unit tests instead of grouped tests

Grouped unit testing can centralize initialization and cleanup operations, which is efficient. However, for components like TcpServer, which frequently starts up and shuts down, singularizing each test scenario into its own method provides more robust, predictable, and reportable behavior.

# Using GoMock for generating mocks for the dependencies

To minimize human error and automate unit test execution, mocks for dependencies are generated using the GoMock library (go.uber.org/mock).

# Security

Due to the scope of the requirements, security measures such as TLS or Encode/Decode has not been implemented.

# Consecutive request handling on an open connection

Server has been designed to process as many consecutive requests as possible, as long as each request is terminated ('\n') correctly. Due to this design, TCP Server Line 179 refreshes the deadline on an active request stream reader, as soon as a correctly escaped request is received.

# Important note about TCP Backlog Queue Management

Integration tests cover various scenarios where the TCP server is shut down, restarted, and more. For tests where the number of incoming requests exceeds the capacity that the OS Network Stack can directly handle, subsequent connections are enqueued in the TCP backlog queue.

Since net.Listener.Accept() (Line 113) is not triggered when a connection is established but placed on hold in the TCP backlog queue until the server has resources to handle it, this can result in a failure scenario (i.e. Ghost Session). If the server receives an interrupt signal via the "exit channel" while there are still connections waiting in the backlog queue, net.Dial may return a connection, but net.Listener.Accept() will be unable to pick it up, causing an abrupt termination.

This scenario is specifically handled on Line 436 and Line 445 of the main_test.go file.