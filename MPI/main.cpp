//#include "stdafx.h"
#include "mpi.h"
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <stdlib.h>
#include <ctype.h>

#define MAX_STDIO_OUTPUT 100

int readFile(char* name, char** buffer);
int str2int(char* str, char** end);

int* computePrimes(int n, int* count, int* basePrimes, int basePrimesCnt, int a, int b);
void print(FILE* file, int* primes, int cnt);

void sort(int* primes, int cnt);

int main(int argc, char* argv[]) {

    MPI_Status status;

    int rank;
    int workersCnt;

    double startTime = 0;
    double endTime = 0;

    // primes till sqrt(b)
    int* basePrimes = NULL;
    int basePrimesCnt = 0;

    // final primes
    int* primes = NULL;
    int primesCnt = 0;

    // used to recieve counts of local primes
    int* localCounts;
    // displastments
    int* displs;

    int a; // low boundary
    int b; // top boundary

    MPI_Init(&argc, &argv);
    MPI_Comm_rank(MPI_COMM_WORLD, &rank);
    MPI_Comm_size(MPI_COMM_WORLD, &workersCnt); 

    if (rank == 0) {
        
        // load input data : int int
        // lazy to do proper error handling and stuff,
        // just a bit of copy-paste
        
        char* str;
        if (readFile((char*) "input.txt", &str)) {
            printf("Error: unable to read file input.txt!\n");
            MPI_Abort(MPI_COMM_WORLD, 1);
        }

        char* end = NULL;
        a = str2int(str, &end);
        if (end == str) {
            printf("Error: worng input format! 'int int' (ex. '12 32') expected!\n");
            MPI_Abort(MPI_COMM_WORLD, 1);
        }

        b = strtol(end + 1, &end, 10);
        if (end == str) {
            printf("Error: worng input format! 'int int' (ex. '12 32') expected!\n");
            MPI_Abort(MPI_COMM_WORLD, 1); 
        }

        if (*end != '\0') {
            printf("Warning: ignoring file information after %i index.\n", end - str);
        }

        if (a > b) {
             printf("Error: low boundary is greater than top one!\n");
             MPI_Abort(MPI_COMM_WORLD, 1);
        }



        // start of computation
        startTime = MPI_Wtime();
        
        if (b < 2) goto theEnd;
        
        basePrimes = computePrimes(floor(sqrt((double) b)), &basePrimesCnt, NULL, 0, 0, 0);
        if (!basePrimes) {
            printf("Error: something went wrong while computing base primes till sqrt(b)!\n");
            MPI_Abort(MPI_COMM_WORLD, 1);
        }

        printf("\n---\n");
        printf("Search range is: [%i, %i]\n", a, b);
        printf("Base prime numbers are:\n");
        print(stdout, basePrimes, basePrimesCnt);
        printf("---\n\n");

        if (basePrimesCnt == 0) goto theEnd;
        
    }



    // send data
    MPI_Bcast(&a, 1, MPI_INT, 0, MPI_COMM_WORLD);
    MPI_Bcast(&b, 1, MPI_INT, 0, MPI_COMM_WORLD);
    MPI_Bcast(&basePrimesCnt, 1, MPI_INT, 0, MPI_COMM_WORLD);

    if (rank != 0 && b < 2) {
        MPI_Finalize();
        return 0;
    }

    if (rank != 0) {
        basePrimes = (int*) malloc(basePrimesCnt * sizeof(int));
        if (!basePrimes) MPI_Abort(MPI_COMM_WORLD, 2);
    }

    MPI_Bcast(basePrimes, basePrimesCnt, MPI_INT, 0, MPI_COMM_WORLD);



    // compute local bounding
    const int lastBasePrime = basePrimes[basePrimesCnt - 1];
    const int n = b - a + 1 ;
    int chunk = n / workersCnt;
    
    int aa; // local low
    int bb; // local top
    
    if (rank != 0) {
        aa = lastBasePrime + a + (rank - 1) * chunk;
        bb = aa + chunk - 1;
    } else {
        aa = lastBasePrime + a + chunk * (workersCnt - 1);
        bb = b;
    }

    const int nn = bb - aa - 1;

    int* localPrimes;
    int localPrimesCnt = 0;
    
    if (nn > 0) {
        localPrimes = computePrimes(nn, &localPrimesCnt, basePrimes, basePrimesCnt, aa, bb);
        if (!localPrimes) {
            printf("Error: something went wrong while using computePrimes!\n");
            MPI_Abort(MPI_COMM_WORLD, 5);
        }
    }


    if (rank == 0) {
        localCounts = (int*) malloc(workersCnt * sizeof(int));
        if (!localCounts) MPI_Abort(MPI_COMM_WORLD, 2);
    }

    MPI_Gather(&localPrimesCnt, 1, MPI_INT, localCounts, 1, MPI_INT, 0, MPI_COMM_WORLD);
    
    if (rank == 0) {
        
        displs = (int*) malloc(workersCnt * sizeof(int));
        
        if (!displs) {
            free(localPrimes);
            MPI_Abort(MPI_COMM_WORLD, 6);
        }

        displs[0] = 0;
        
        primesCnt += localCounts[0];
        for (int i = 1; i < workersCnt; i++) {
            primesCnt += localCounts[i];
            displs[i] = displs[i-1] + localCounts[i-1]; // edit TODO
        }


        if (primesCnt > 0) {
            primes = (int*) malloc(primesCnt * sizeof(int));
            if (!primes) {
                free(localCounts);
                free(displs);
                MPI_Abort(MPI_COMM_WORLD, 8);
            }
        }
    
    }

    MPI_Gatherv(localPrimes,
                localPrimesCnt,
                MPI_INT,
                primes,         // target buffer
                localCounts,    // local answer
                displs,         // displacement array
                MPI_INT,
                0,              // root process
                MPI_COMM_WORLD
    );



    theEnd:
    if (rank == 0) {

        const double elapsedTime = MPI_Wtime() - startTime;

        sort(primes, primesCnt);

        FILE* file = fopen((char*) "output.txt", "w");
        if (!file) {
            printf("Error: could not open output.txt!\n");
            MPI_Abort(MPI_COMM_WORLD, 1);
        }

        printf("Elapsed time: %fs\n", elapsedTime);
        printf("There are %i prime numbers in range [%i, %i] (%i shown):\n", basePrimesCnt + primesCnt, a, b, MAX_STDIO_OUTPUT);        
        if (MAX_STDIO_OUTPUT < basePrimesCnt) {
            print(stdout, basePrimes, MAX_STDIO_OUTPUT);
            printf("...\n");
        } else if (MAX_STDIO_OUTPUT < basePrimesCnt + primesCnt) {
            print(stdout, basePrimes, basePrimesCnt);
            print(stdout, primes, MAX_STDIO_OUTPUT - basePrimesCnt);
            printf("...\n");
        } else {
            print(stdout, basePrimes, basePrimesCnt);
            print(stdout, primes, primesCnt);
        }

        print(file, basePrimes, basePrimesCnt);
        print(file, primes, primesCnt);
        
        fclose(file);

    }

    MPI_Finalize();
    return 0;

}

int readFile(char* name, char** buffer) {

    FILE* file = fopen(name, "rb");
    if (!file) return 1;

    fseek(file, 0, SEEK_END);
	const int fileSize = ftell(file);
	fseek(file, 0, SEEK_SET);

    *buffer = (char*) malloc(fileSize + 1);
    if (!*buffer) return 1;

    fread(*buffer, 1, fileSize + 1, file); 
    (*buffer)[fileSize] = '\0';

    fclose(file);
    return 0;

}

int str2int(char* str, char** end) {

    if (*str == '\0' || isspace((unsigned char) *str)) {
        *end = str;
        return 0;
    }

    errno = 0;
    long num = strtol(str, end, 10);
    if (errno ==  ERANGE) {
        *end = str;
        return 0;
    }

    return num;

}

// computes primes till 1 to n (inclussive)
int* computePrimes(int n, int* count, int* basePrimes, int basePrimesCnt, int a, int b) {
    
    if (n < 2) return NULL;

    char* isPrime = (char*) calloc(1, n * sizeof(char));
    if (!isPrime) return NULL;

    if (!basePrimes) {
        
        // 0: prime, 1: non prime
        // mark all multiples as non prime
        for (int p = 2; p * p <= n; p++) {
            if (isPrime[p - 1]) continue;
            for (int i = p * p; i <= n; i += p) {
                isPrime[i - 1] = 1;
            }
        }

    } else {
        
        for (int i = 0; i < basePrimesCnt; i++) {
            
            int p = basePrimes[i];
            if (p * p > b || p == 1) continue;

            // multiple of p >= a
            // a % p;
            int mp = (a + p - 1) / p * p;

            // go through all multiples
            for (int i = mp; i <= b; i += p) {
                isPrime[i - a] = 1;
            }
        
        }

        a -= 1;

    }

    int primeCount = 0;
    for (int i = 0; i < n; i++) {
        if (!isPrime[i]) primeCount++;
    }

    int* primes = (int*) malloc(primeCount * sizeof(int));
    if (!primes) {
        free(isPrime);
        return NULL;
    }

    int p = 0;
    for (int i = 0; i < n; i++) {
        if (!isPrime[i]) {
            primes[p] = a + i + 1;
            p++;
        }
    }

    free(isPrime);
    *count = primeCount;
    
    return primes;

}

void print(FILE* file, int* primes, int cnt) {
    for (int i = 0; i < cnt; i++) {
        fprintf(file, "%i\n", primes[i]);
    }
}

void swap(int *a, int *b) {
    const int t = *a;
    *a = *b;
    *b = t;
}

int partition(int array[], const int low, const int high) {
  
    const int pivot = array[high];
    int i = (low - 1);
    for (int j = low; j < high; j++) {
        if (array[j] <= pivot) {
        i++;
        swap(&array[i], &array[j]);
        }
    }

    swap(&array[i + 1], &array[high]);
  
    return (i + 1);

}

void quickSort(int array[], const int low, const int high) {
  
    if (low < high) {
        const int pi = partition(array, low, high);
        quickSort(array, low, pi - 1);
        quickSort(array, pi + 1, high);
    }

}

void sort(int* primes, int cnt) {
    quickSort(primes, 0, cnt - 1);
}
